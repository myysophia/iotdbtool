package cmd

import (
	"fmt"
	_ "path/filepath"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/tools/clientcmd"
)

var (
	restoreFile string
)

func init() {
	restoreCmd.Flags().StringVar(&restoreFile, "file", "", "要恢复的文件名")
	restoreCmd.Flags().StringSliceVar(&pods, "pods", []string{}, "Comma-separated list of pod names")
	restoreCmd.Flags().StringVarP(&label, "label", "l", "", "Label selector to filter pods")
	restoreCmd.Flags().StringVarP(&dataDir, "datadir", "d", "/iotdb/data/", "Data directory inside the pod")
	restoreCmd.Flags().StringVarP(&outName, "outname", "o", "", "Output file name for the backup")
	restoreCmd.Flags().StringVarP(&bucketName, "bucketname", "b", "", "OSS bucket name")
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
	Short: "从 OSS 恢复 IoTDB 数据",
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
			err := restorePod(clientset, pod, restoreFile)
			if err != nil {
				fmt.Printf("恢复 pod %s 失败: %v\n", pod.Name, err)
			} else {
				fmt.Printf("成功恢复 pod %s\n", pod.Name)
			}
		}
	},
}

func restorePod(clientset *kubernetes.Clientset, pod v1.Pod, fileName string) error {
	containerList := strings.Split(containers, ",")
	for _, containerName := range containerList {
		containerName = strings.TrimSpace(containerName)
		fmt.Printf("正在处理 pod %s 的容器 %s\n", pod.Name, containerName)

		// 下载文件从 OSS
		if err := downloadFromOSS(clientset, pod.Name, containerName, fileName); err != nil {
			return fmt.Errorf("从 OSS 下载文件失败: %v", err)
		}

		// 解压文件并执行恢复命令
		restoreCmd := fmt.Sprintf(`
			tar -xf %s && 
			find iotdb/data/datanode/ -name "*.tsfile" | 
			xargs -I GG echo "/iotdb/sbin/start-cli.sh -h iotdb-datanode-0 -e \"load 'GG' verify=false \";" | 
			bash
		`, fileName)

		_, err := executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", restoreCmd}, configPath)
		if err != nil {
			return fmt.Errorf("执行恢复命令失败: %v", err)
		}

		// 删除下载的文件
		deleteCmd := fmt.Sprintf("rm -f %s", fileName)
		_, err = executePodCommand(clientset, namespace, pod.Name, containerName, []string{"sh", "-c", deleteCmd}, configPath)
		if err != nil {
			fmt.Printf("警告：删除下载的文件 %s 失败: %v\n", fileName, err)
		}
	}

	return nil
}

func downloadFromOSS(clientset *kubernetes.Clientset, podName, containerName, fileName string) error {
	credentials, err := loadCredentials(".credentials")
	if err != nil {
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
