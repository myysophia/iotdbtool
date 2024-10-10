package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/tools/clientcmd"
	_ "path/filepath"
	"strings"
	"sync"
)

var (
	restoreFile string
)

func init() {
	restoreCmd.Flags().StringVar(&restoreFile, "file", "", "要恢复的文件名")
	restoreCmd.Flags().StringSliceVar(&pods, "pods", []string{}, "Comma-separated list of pod names")
	restoreCmd.Flags().StringVarP(&label, "label", "l", "", "Label selector to filter pods")
	restoreCmd.Flags().StringVarP(&dataDir, "datadir", "d", "/iotdb/data/datanode", "Data directory inside the pod")
	restoreCmd.Flags().StringVarP(&outName, "outname", "o", "", "Output file name for the backup")
	restoreCmd.Flags().StringVarP(&bucketName, "bucketname", "b", "iotdb-backup", "OSS bucket name")
	restoreCmd.Flags().IntVarP(&verbose, "verbose", "v", 0, "Verbose level (0: silent, 1: basic, 2: detailed)")
	restoreCmd.Flags().StringVar(&configPath, "config", "/root/.kube/config", "Path to the kubeconfig file")
	restoreCmd.Flags().StringVar(&namespace, "namespace", "default", "Kubernetes namespace")
	restoreCmd.Flags().BoolVar(&keepLocal, "keep-local", true, "保留本地备份文件")
	restoreCmd.Flags().Int64Var(&chunkSize, "chunksize", 10*1024*1024, "下载和上传的分片大小（字节）")
	restoreCmd.Flags().StringVar(&containers, "containers", "iotdb-datanode", "要操作的容器，多个容器用逗号分隔")
	rootCmd.AddCommand(restoreCmd)
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "restore iotdb data from OSS ",
	Long:  `从 OSS 下载备份文件并恢复到指定的 Kubernetes pods 中。`,
	Run: func(cmd *cobra.Command, args []string) {
		if restoreFile == "" {
			fmt.Println("错误：必须指定要恢复的文件名（使用 --file 参数）")
			return
		}

		clientset, err := getClientSet(configPath)
		if err != nil {
			fmt.Printf("创建 Kubernetes 客户端失败: %v\n", err)
			return
		}

		podList, err := getPodList(clientset, namespace, pods, "")
		if err != nil {
			fmt.Printf("获取 pod 列表失败: %v\n", err)
			return
		}

		for _, pod := range podList.Items {
			trackStepDuration("restore by load tsfile", func() error {
				return restorePod(clientset, pod, restoreFile, pods)
			})
		}
	},
}

//func trackStepDuration(stepName string, stepFunc func() error) {
//	startTime := time.Now()
//	err := stepFunc()
//	duration := time.Since(startTime)
//	if err != nil {
//		log(0, "%s failed: %v", stepName, err)
//	} else {
//		log(1, "%s successful。durtions: %v", stepName, duration)
//	}
//}

func restorePod(clientset *kubernetes.Clientset, pod v1.Pod, fileName string, pods []string) error {
	containerList := strings.Split(containers, ",")
	var wg sync.WaitGroup // 使用 WaitGroup 等待所有 goroutine 完成

	for _, containerName := range containerList {
		containerName = strings.TrimSpace(containerName)
		fmt.Printf("正在处理 pod %s 的容器 %s\n", pod.Name, containerName)

		trackStepDuration("env check", func() error {
			return ensureOssutilAvailable(clientset, namespace, pod.Name, containerName, configPath)
		})

		// 下载文件从 OSS
		trackStepDuration("download from oss", func() error {
			return downloadFromOSS(clientset, pod.Name, containerName, fileName)
		})

		// 解压文件并获取 tsfile
		restoreCmd := fmt.Sprintf("tar -xf %s && find iotdb/data/datanode/ -name \"*.tsfile\"", fileName)
		log(2, "执行解压命令: %s", restoreCmd)
		_, err := executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", restoreCmd}, configPath)
		if err != nil {
			return fmt.Errorf("解压命令失败: %v", err)
		}

		// 获取 tsfile 列表
		tsfileCmd := "find iotdb/data/datanode/ -name \"*.tsfile\""
		tsfileList, err := executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", tsfileCmd}, configPath)
		if err != nil {
			return fmt.Errorf("获取 tsfile 列表失败: %v", err)
		}

		// 拆分 tsfile 列表并并发执行 load 命令
		tsfiles := strings.Split(tsfileList, "\n")
		for _, tsfile := range tsfiles {
			if tsfile == "" {
				continue
			}
			wg.Add(1) // 增加 WaitGroup 计数
			go func(tsfile string) {
				defer wg.Done() // 完成时减少计数
				loadCmd := fmt.Sprintf("/iotdb/sbin/start-cli.sh -h %s -e \"load '%s' verify=false\";", pod.Name, tsfile)
				log(2, "执行加载命令: %s", loadCmd)
				_, err := executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", loadCmd}, configPath)
				if err != nil {
					fmt.Printf("加载命令失败: %v\n", err)
				}
			}(tsfile) // 传递 tsfile
		}
	}

	wg.Wait() // 等待所有 goroutine 完成

	// 删除文件
	//deleteCmd := fmt.Sprintf("rm -rf ./iotdb")
	//_, err := executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", deleteCmd}, configPath)
	//if err != nil {
	//	fmt.Printf("警告：删除下载的文件 %s 失败: %v\n", fileName, err)
	//}

	return nil
}

func downloadFromOSS(clientset *kubernetes.Clientset, podName, containerName, fileName string) error {
	credentials, err := loadCredentials(".credentials")
	if err != nil {
		log(2, "加载 OSS 凭证失败: %v", err)
		return err
	}

	// 创建 ossutil 配置文件
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

	// 使用 ossutil 下载文件
	downloadCmd := fmt.Sprintf(`
		./ossutil64 cp oss://%s/%s %s \
		-c %s \
		--force
	`, bucketName, fileName, fileName, configFileName)

	_, stderr, err := executePodCommandWithStderr(clientset, namespace, podName, containerName, []string{"sh", "-c", downloadCmd}, configPath)
	log(2, "下载文件命令的 downloadCmd: %s", downloadCmd)
	if err != nil {
		return fmt.Errorf("从 OSS 下载文件失败: %v, stderr: %s", err, stderr)
	}

	// 删除配置文件
	deleteConfigCmd := fmt.Sprintf("rm -f %s", configFileName)
	_, err = executePodCommand(clientset, namespace, podName, containerName, []string{"sh", "-c", deleteConfigCmd}, configPath)
	if err != nil {
		fmt.Printf("警告: 删除 ossutil 配置文件失败: %v\n", err)
	}

	return nil
}
