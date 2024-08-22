package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	//"strconv"
	"strings"
	//"sync"
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
	keepLocal  bool
	chunkSize  int64
	containers string
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
	backupCmd.Flags().BoolVar(&keepLocal, "keep-local", true, "保留本地备份文件")
	backupCmd.Flags().Int64Var(&chunkSize, "chunksize", 10*1024*1024, "下载和上传的分片大小（字节）")
	backupCmd.Flags().StringVar(&containers, "containers", "iotdb-datanode", "要操作的容器，多个容器用逗号分隔")

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

	containerList := strings.Split(containers, ",")
	for _, container := range containerList {
		container = strings.TrimSpace(container)
		log(1, "正在处理容器: %s", container)

		trackStepDuration("确保 ossutil64 可用", func() error {
			return ensureOssutilAvailable(clientset, namespace, pod.Name, container, configPath)
		})

		trackStepDuration("刷新数据", func() error {
			return flushData(clientset, namespace, pod.Name, container, configPath)
		})

		backupFileName := getBackupFileName(pod.Name, container, outName)
		trackStepDuration("压缩数据", func() error {
			return compressData(clientset, namespace, pod.Name, dataDir, backupFileName, container, configPath, outName)
		})

		trackStepDuration("上传到OSS并删除Pod中的文件", func() error {
			err := uploadToOSSFromPod(clientset, namespace, pod.Name, backupFileName, container, bucketName, configPath)
			if err != nil {
				log(2, "上传到OSS并删除Pod中的文件失败: %v", err)
				return err
			}
			return deletePodFile(clientset, namespace, pod.Name, backupFileName, container, configPath)
		})
	}

	podEndTime := time.Now()
	log(2, "pod %s 的备份完成。耗时: %v", pod.Name, podEndTime.Sub(podStartTime))
}

func ensureOssutilAvailable(clientset *kubernetes.Clientset, namespace, podName, containerName, configPath string) error {
	// 检查 ossutil64 是否已存在
	checkCmd := "if [ -f ./ossutil64 ] && [ -x ./ossutil64 ]; then echo 'exists'; else echo 'not found'; fi"
	output, err := executePodCommand(clientset, namespace, podName, containerName, []string{"sh", "-c", checkCmd}, configPath)
	if err != nil {
		return fmt.Errorf("检查 ossutil64 是否存在失败: %v", err)
	}

	if strings.TrimSpace(output) == "exists" {
		log(2, "ossutil64 已存在于 pod %s 中", podName)
		return nil
	}

	// 如果不存在，下载并安装 ossutil64
	log(2, "正在下载并安装 ossutil64 到 pod %s", podName)
	downloadCmd := `
		curl -o ossutil64 http://gosspublic.alicdn.com/ossutil/1.7.7/ossutil64 && \
		chmod 755 ossutil64
	`
	_, err = executePodCommand(clientset, namespace, podName, containerName, []string{"sh", "-c", downloadCmd}, configPath)
	if err != nil {
		return fmt.Errorf("下载并安装 ossutil64 失败: %v", err)
	}

	log(2, "已在 pod %s 中下载并安装 ossutil64", podName)
	return nil
}

func uploadToOSSFromPod(clientset *kubernetes.Clientset, namespace, podName, fileName, containerName, bucketName, configPath string) error {
	credentials, err := loadCredentials(".credentials")
	if err != nil {
		return err
	}

	// 首先创建 ossutil 配置文件
	configContent := fmt.Sprintf(`
[Credentials]
language=EN
endpoint=%s
accessKeyID=%s
accessKeySecret=%s
`, credentials["ENDPOINT"], credentials["AK"], credentials["SK"])

	configFileName := ".ossutilconfig"
	createConfigCmd := fmt.Sprintf(`echo '%s' > %s`, configContent, configFileName)

	_, err = executePodCommand(clientset, namespace, podName, containerName, []string{"sh", "-c", createConfigCmd}, configPath)
	if err != nil {
		return fmt.Errorf("创建 ossutil 配置文件失败: %v", err)
	}

	// 使用配置文件上传
	uploadCmd := fmt.Sprintf(`
		./ossutil64 cp %s oss://%s/ \
		-c %s \
		--force
	`, fileName, bucketName, configFileName)

	log(2, "上传文件到 OSS 命令: %s", uploadCmd)
	// 在 pod 中执行上传命令
	cmd := []string{"sh", "-c", uploadCmd}

	stdout, stderr, err := executePodCommandWithStderr(clientset, namespace, podName, containerName, cmd, configPath)
	if err != nil {
		return fmt.Errorf("上传文件到 OSS 失败: %v, stderr: %s", err, stderr)
	}

	// 删除配置文件
	deleteConfigCmd := fmt.Sprintf("rm -f %s", configFileName)
	_, err = executePodCommand(clientset, namespace, podName, containerName, []string{"sh", "-c", deleteConfigCmd}, configPath)
	if err != nil {
		log(2, "警告: 删除 ossutil 配置文件失败: %v", err)
	}

	log(2, "文件 %s 已从 pod %s 上传到 OSS。输出: %s", fileName, podName, stdout)
	return nil
}

func deletePodFile(clientset *kubernetes.Clientset, namespace, podName, fileName, containerName, configPath string) error {
	cmd := []string{"rm", "-f", fileName}

	_, err := executePodCommand(clientset, namespace, podName, containerName, cmd, configPath)
	if err != nil {
		return fmt.Errorf("删除 pod 中的文件失败: %v", err)
	}

	log(2, "已删除 pod %s 中的文件: %s", podName, fileName)
	return nil
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

func flushData(clientset *kubernetes.Clientset, namespace, podName, containerName, configPath string) error {
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
			Container: containerName,
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

	log(2, "Flush command output: %s", stdout.String())

	time.Sleep(5 * time.Second)
	return nil
}

func compressData(clientset *kubernetes.Clientset, namespace, podName, dataDir, outputFileName, containerName, configPath, outName string) error {
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
			Container: containerName,
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

func getFileSizeFromPod(clientset *kubernetes.Clientset, namespace, podName, containerName, fileName, configPath string) (int64, error) {
	cmd := []string{"stat", "-c", "%s", fileName}

	output, err := executePodCommand(clientset, namespace, podName, containerName, cmd, configPath)
	if err != nil {
		return 0, err
	}

	size, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析文件大小: %v", err)
	}

	return size, nil
}

func executePodCommand(clientset *kubernetes.Clientset, namespace, podName, containerName string, cmd []string, configPath string) (string, error) {
	kubeconfigPath := configPath
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("error building config from kubeconfig: %v", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("error creating executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return "", fmt.Errorf("error executing command: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func executePodCommandWithStderr(clientset *kubernetes.Clientset, namespace, podName, containerName string, cmd []string, configPath string) (string, string, error) {
	kubeconfigPath := configPath
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", "", fmt.Errorf("error building config from kubeconfig: %v", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("error creating executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	return stdout.String(), stderr.String(), err
}

func getBackupFileName(podName, containerName, customName string) string {
	if customName != "" {
		return fmt.Sprintf("%s_%s_%s_%s.tar.gz", customName, podName, containerName, time.Now().Format("20060102150405"))
	}
	return fmt.Sprintf("%s_%s_%s.tar.gz", podName, containerName, time.Now().Format("20060102150405"))
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

	// 设置分片大小为配置的 chunkSize
	partSize := chunkSize

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
		fileName+" 正在上传",
	)

	// 分片上传
	var parts []oss.UploadPart
	for i := int64(0); i < fileSize; i += partSize {
		end := i + partSize
		if end > fileSize {
			end = fileSize
		}
		partSize := end - i

		// 创建一个制读取大小的 Reader
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

	log(2, "文件 %s 已上传到OSS，将在 %s 后自动删除", fileName, expirationTime.Format("2006-01-02 15:04:05"))
	return nil
}

func deleteLocalFile(fileName string) error {
	err := os.Remove(fileName)
	if err != nil {
		return fmt.Errorf("删除本地文件 %s 失败: %v", fileName, err)
	}
	log(2, "本地文件 %s 已删除", fileName)
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
