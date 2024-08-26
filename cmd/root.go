package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "iotdbbackuprestore",
	Short: "A tool for back up and restore IoTDB data for nova-ems",
	Long:  `iotdbbackuprestore is a CLI tool to backup and restore IoTDB data in Kubernetes.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// 可以在这里添加全局命令标志，或者初始化子命令
	rootCmd.PersistentFlags().StringP("config", "c", "/root/.kube/config", "Path to the kubeconfig file")
	rootCmd.PersistentFlags().StringP("namespace", "n", "iotdb", "Kubernetes namespace")
	rootCmd.PersistentFlags().StringP("pods", "p", "iotdb-datanode-0", "backup by pod name")
	rootCmd.PersistentFlags().StringP("label", "l", "statefulset.kubernetes.io/pod-name=iotdb-datanode-0", "backup by pod label")
	rootCmd.PersistentFlags().StringP("datadir", "d", "/iotdb/data", "iotdb data dir")
	rootCmd.PersistentFlags().StringP("outname", "o", "iotdb-datanode-back", "backup file name")
	rootCmd.PersistentFlags().StringP("bucketname", "b", "iotdb-backup", "oss bucket name")
	rootCmd.PersistentFlags().StringP("verbose", "v", "0", "backup log level")
	rootCmd.PersistentFlags().StringP("keep-local", "k", "true", "keep file to local")
	rootCmd.PersistentFlags().StringP("chunksize", "s", "10485760", "default chunksize is 10MB")
	rootCmd.PersistentFlags().StringP("containers", "t", "iotdb-datanode", "default container")
	rootCmd.PersistentFlags().StringP("cluster-name", "m", "", "k8s clusterName")
	rootCmd.PersistentFlags().StringP("uploadoss", "", "yes", "uploadoss flag，default is true")
}
