package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "iotdbtools",
	Short: "A tool for back up and restore IoTDB data for nova-ems",
	Long:  `iotdbtools is a CLI tool to backup and restore IoTDB data in Kubernetes.`,
}

// 定义 completion 子命令
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh]",
	Short: "Generate the autocompletion script for the specified shell",
	Long: `To load completions:

Bash:
  $ source <(iotdbtools completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ iotdbtools completion bash > /etc/bash_completion.d/iotdbtools
  # macOS:
  $ iotdbtools completion bash > /usr/local/etc/bash_completion.d/iotdbtools

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ iotdbtools completion zsh > "${fpath[1]}/_iotdbtools"

  # You will need to start a new shell for this setup to take effect.`,
	Args:      cobra.ExactValidArgs(1),
	ValidArgs: []string{"bash", "zsh"},
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			rootCmd.GenZshCompletion(os.Stdout)
		default:
			cmd.Println("Only bash and zsh completion are supported.")
		}
	},
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
	rootCmd.PersistentFlags().StringP("datadir", "d", "/iotdb/data/datanode", "iotdb data dir")
	rootCmd.PersistentFlags().StringP("outname", "o", "iotdb-datanode-back", "backup file name")
	rootCmd.PersistentFlags().StringP("bucketname", "b", "iotdb-backup", "oss bucket name")
	rootCmd.PersistentFlags().StringP("verbose", "v", "0", "backup log level")
	rootCmd.PersistentFlags().StringP("keep-local", "k", "true", "keep file to local")
	rootCmd.PersistentFlags().StringP("chunksize", "s", "10485760", "default chunksize is 10MB")
	rootCmd.PersistentFlags().StringP("containers", "t", "iotdb-datanode", "default container")
	rootCmd.PersistentFlags().StringP("cluster-name", "m", "", "k8s clusterName")
	rootCmd.PersistentFlags().StringP("uploadoss", "", "yes", "uploadoss flag，default is true")
	rootCmd.PersistentFlags().StringP("osscong", "", ".iotdbtoolss.conf", "oss config file")
	// 添加 completion 子命令
	rootCmd.AddCommand(completionCmd)
	//rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
	//	if ossConf {
	//		configureOSS()
	//	}
	//	checkOSSConfig()
	//}
}
