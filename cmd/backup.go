package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	pods       []string
	label      string
	dataDir    string
	outName    string
	bucketName string
	verbose    int
	configPath string
	namespace  string
)

func init() {
	backupCmd.Flags().StringSliceVar(&pods, "pods", []string{}, "Comma-separated list of pod names")
	backupCmd.Flags().StringVarP(&label, "label", "l", "", "Label selector to filter pods")
	backupCmd.Flags().StringVarP(&dataDir, "datadir", "d", "/iotdb/data/", "Data directory inside the pod")
	backupCmd.Flags().StringVarP(&outName, "outname", "o", "", "Output file name for the backup")
	backupCmd.Flags().StringVarP(&bucketName, "bucketname", "b", "", "OSS bucket name")
	backupCmd.Flags().IntVarP(&verbose, "verbose", "v", 0, "Verbose level (0: silent, 1: basic, 2: detailed)")
	backupCmd.Flags().StringVar(&configPath, "config", "", "Path to the kubeconfig file")
	backupCmd.Flags().StringVar(&namespace, "namespace", "default", "Kubernetes namespace")

	rootCmd.AddCommand(backupCmd)
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup IoTDB data",
	Long:  `Backup IoTDB data from Kubernetes pods and upload to OSS.`,
	Run: func(cmd *cobra.Command, args []string) {
		startTime := time.Now()
		log(1, "开始时间: %s", startTime.Format("2006-01-02 15:04:05"))

		clientset, err := getClientSet(configPath)
		if err != nil {
			log(0, "创建 Kubernetes 客户端失败: %v", err)
			os.Exit(1)
		}

		podList, err := getPodList(clientset, namespace, pods, label)
		if err != nil {
			log(0, "列出 pods 失败: %v", err)
			os.Exit(1)
		}

		// 使用 goroutine 和 channel 并行处理 pod 备份
		podCount := len(podList.Items)
		doneChan := make(chan bool, podCount)

		for _, pod := range podList.Items {
			go func(pod v1.Pod) {
				backupPod(clientset, pod)
				doneChan <- true
			}(pod)
		}

		// 等待所有 pod 备份完成
		for i := 0; i < podCount; i++ {
			<-doneChan
		}

		endTime := time.Now()
		log(1, "结束时间: %s", endTime.Format("2006-01-02 15:04:05"))
		log(1, "总耗时: %v", endTime.Sub(startTime))
	},
}

func backupPod(clientset *kubernetes.Clientset, pod v1.Pod) {
	podStartTime := time.Now()
	log(1, "正在处理 pod: %s", pod.Name)

	trackStepDuration("刷新数据", func() error {
		return flushData(clientset, namespace, pod.Name, configPath)
	})

	backupFileName := getBackupFileName(pod.Name, outName)
	trackStepDuration("压缩数据", func() error {
		log(2, "开始压缩数据到 %s", backupFileName)
		return compressData(clientset, namespace, pod.Name, dataDir, backupFileName, configPath)
	})

	trackStepDuration("下载文件", func() error {
		log(2, "开始下载文件 %s 到本地", backupFileName)
		return downloadFileFromPod(clientset, namespace, pod.Name, backupFileName, "iotdb-datanode", outName, backupFileName, configPath)
	})

	trackStepDuration("上传到OSS", func() error {
		log(2, "开始上传文件 %s 到 OSS", backupFileName)
		return uploadToOSS(backupFileName, bucketName)
	})

	podEndTime := time.Now()
	log(1, "pod %s 的备份完成。耗时: %v", pod.Name, podEndTime.Sub(podStartTime))
}

func getClientSet(kubeconfig string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func trackStepDuration(stepName string, stepFunc func() error) {
	startTime := time.Now()
	err := stepFunc()
	duration := time.Since(startTime)
	if err != nil {
		log(0, "%s 失败: %v", stepName, err)
	} else {
		log(1, "%s 完成。耗时: %v", stepName, duration)
	}
}

func getPodList(clientset *kubernetes.Clientset, namespace string, pods []string, label string) (*v1.PodList, error) {
	var options metav1.ListOptions

	if label != "" {
		options.LabelSelector = label
	}

	if len(pods) > 0 {
		podList := &v1.PodList{
			Items: []v1.Pod{},
		}
		for _, podName := range pods {
			pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
			if err != nil {
				log(0, "获取 pod %s 失败: %v", podName, err)
				continue
			}
			podList.Items = append(podList.Items, *pod)
		}
		return podList, nil
	}

	return clientset.CoreV1().Pods(namespace).List(context.TODO(), options)
}

func flushData(clientset *kubernetes.Clientset, namespace, podName string, configPath string) error {
	cmd := []string{"/iotdb/sbin/start-cli.sh", "-h", "iotdb-datanode", "-e", "flush on cluster"}
	kubeconfigPath := configPath
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("error building config from kubeconfig: %v", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Name(podName).
		Namespace(namespace).
		Resource("pods").
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: "iotdb-datanode",
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error creating executor: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	if err != nil {
		return fmt.Errorf("error streaming command: %v, stderr: %s", err, stderr.String())
	}

	log(1, "Flush command output: %s", stdout.String())

	time.Sleep(5 * time.Second)
	return nil
}

func compressData(clientset *kubernetes.Clientset, namespace, podName, dataDir, outputFileName string, configPath string) error {
	cmd := []string{"tar", "-czf", outputFileName, dataDir}
	kubeconfigPath := configPath
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("error building config from kubeconfig: %v", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Name(podName).
		Namespace(namespace).
		Resource("pods").
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: "iotdb-datanode",
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error creating executor: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	if err != nil {
		return fmt.Errorf("error streaming command: %v, stderr: %s", err, stderr.String())
	}

	log(1, "Compression command output: %s", stdout.String())
	log(2, "压缩文件 %s 成功", outputFileName)
	return nil
}

func downloadFileFromPod(clientset *kubernetes.Clientset, namespace, podName, backupFileName, containerName, remoteFilePath, localFilePath string, configPath string) error {
	cmd := []string{"tar", "cf", "-", backupFileName}
	kubeconfigPath := configPath

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("error building config from kubeconfig: %v", err)
	}

	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Command:   cmd,
			Container: containerName,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error creating executor: %v", err)
	}
	log(2, " pod %s 开始解压缩到local %s", podName, backupFileName)

	// 打开本地文件以写入下载内容
	file, err := os.Create(localFilePath)
	if err != nil {
		return fmt.Errorf("error creating local file: %v", err)
	}
	defer file.Close()

	// 将文件内容从 pod 传输到本地文件
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: file,
		Stderr: os.Stderr,
	})
	if err != nil {
		return fmt.Errorf("error executing stream: %v", err)
	}
	log(2, "开始从 pod %s 下载文件 %s 到本地", podName, backupFileName)
	return nil
}

func getBackupFileName(podName, customName string) string {
	if customName != "" {
		return fmt.Sprintf("%s_%s_%s.tar.gz", customName, podName, time.Now().Format("20060102150405"))
	}
	return fmt.Sprintf("%s_%s_%s.tar.gz", customName, podName, time.Now().Format("20060102150405"))
}

func uploadToOSS(fileName, bucketName string) error {
	credentials, err := loadCredentials(".credentials")
	if err != nil {
		return err
	}

	client, err := oss.New(credentials["ENDPOINT"], credentials["AK"], credentials["SK"])
	if err != nil {
		return err
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return err
	}

	// 设置对象的过期时间为7天后
	expirationTime := time.Now().AddDate(0, 0, 7)
	options := []oss.Option{
		oss.Expires(expirationTime),
		oss.Routines(3),
	}

	// 获取文件大小
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %v", err)
	}
	fileSize := fileInfo.Size()

	// 设置分片大小为5MB
	partSize := int64(5 * 1024 * 1024)

	// 打开文件
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 初始化分片上传
	imur, err := bucket.InitiateMultipartUpload(fileName, options...)
	if err != nil {
		return fmt.Errorf("初始化分片上传失败: %v", err)
	}

	// 创建进度条
	bar := progressbar.DefaultBytes(
		fileSize,
		"正在上传",
	)

	// 分片上传
	var parts []oss.UploadPart
	for i := int64(0); i < fileSize; i += partSize {
		end := i + partSize
		if end > fileSize {
			end = fileSize
		}
		partSize := end - i

		// 创建一个限制读取大小的 Reader
		partReader := io.LimitReader(file, partSize)

		part, err := bucket.UploadPart(imur, partReader, partSize, int(i/partSize)+1)
		if err != nil {
			bucket.AbortMultipartUpload(imur)
			return fmt.Errorf("上传分片失败: %v", err)
		}
		parts = append(parts, part)
		bar.Add(int(partSize))
	}

	// 完成分片上传
	_, err = bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		return fmt.Errorf("完成分片上传失败: %v", err)
	}

	log(1, "文件 %s 已上传到OSS，将在 %s 后自动删除", fileName, expirationTime.Format("2006-01-02 15:04:05"))
	return nil
}

func loadCredentials(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	creds := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			creds[parts[0]] = parts[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return creds, nil
}

func log(level int, format string, args ...interface{}) {
	if verbose >= level {
		fmt.Printf(format+"\n", args...)
	}
}
