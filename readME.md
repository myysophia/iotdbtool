# iotdbtool
## 项目简介
iotdbtool 是一个使用 Go 语言编写的命令行工具，基于 Kubernetes 环境，提供了 IoTDB 数据的备份功能。它可以从 Kubernetes 集群中的 IoTDB Pod 中提取数据，并将其上传到阿里云 OSS 存储桶中。

iotdbtool 支持 iotDB 单机、集群，备份与恢复，备份文件存储在 oss 上，主要实现了 k8s 部署的有状态服务的备份恢复

## 痛点

- 开源版本 iotdb 没有现成的冷备方案，exporttsfile 基本不可用，引用几十个 jar 导出全库需要巨量内存。
- 业务核心组件，iotdb down = 业务不可用 down。 每天 2 次冷备，以备不时之需
- 多个 iotdb 节点，备份恢复没有一个 all in one 简单的工具

## 功能特性

- 支持任意 Pod 任意容器中的指定目录。
- 将备份文件上传到阿里云 OSS，默认保存 7 天
- 支持多种日志输出级别，便于调试和监控。
- 支持 iotdb 单机、集群的备份和恢复
- goroutine 支持并发备份
- oss sdk 上传 oss、分片上传、进度条
- 不依赖 kubectl 命令，使用 client-go 直接调用 api 操作 pod，安全高效

## 系统要求
Go 1.16 或更高版本
Kubernetes 集群
阿里云 OSS 存储桶和相应的访问权限
## 安装
### 从源码构建
首先，确保你已经安装了 Go 语言开发环境。然后，克隆项目并编译二进制文件：

```bash
git clone http://git.novatools.vip/Nova006393/iotdbtool.git
cd iotdbtool
go build -o iotdbtools
```

### 交叉编译（在 Windows 上编译 Linux 二进制文件）
```bash
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -o iotdbbackup


# linux 上build 语法
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o iotdbtools
```

 设置为 0，可以避免对系统上 C 库的依赖，从而生成更加通用的二进制文件。

编译完成后，你将获得一个 iotdbbackup 二进制文件。可以到处运行

## 使用指南
### backup
![backup](https://github.com/user-attachments/assets/e24fce0a-b7b9-422b-a5b2-5b1aa5add0ef)

### restore 

```bash
./iotdbtools restore --config /opt/kubeconfig/eks-config-ems-eu-newaccount  --namespace iotdbtest --pods=iotdb-datanode-0 --bucketname iotdb-backup --verbose 1 --file emsdev_iotdb-datanode-0_20240830152642.tar.gz
```



### 基本用法
```bash
iotdbtools is a CLI tool to backup and restore IoTDB data in Kubernetes.

Usage:
  iotdbtools [command]

Available Commands:
  backup      Backup IoTDB data
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  restore     restore iotdb data from OSS

Flags:
  -b, --bucketname string     oss bucket name (default "iotdb-backup")
  -s, --chunksize string      default chunksize is 10MB (default "10485760")
  -m, --cluster-name string   k8s clusterName
  -c, --config string         Path to the kubeconfig file (default "/root/.kube/config")
  -t, --containers string     default container (default "iotdb-datanode")
  -d, --datadir string        iotdb data dir (default "/iotdb/data")
  -h, --help                  help for iotdbbackuprestore
  -k, --keep-local string     keep file to local (default "true")
  -l, --label string          backup by pod label (default "statefulset.kubernetes.io/pod-name=iotdb-datanode-0")
  -n, --namespace string      Kubernetes namespace (default "iotdb")
  -o, --outname string        backup file name (default "iotdb-datanode-back")
  -p, --pods string           backup by pod name (default "iotdb-datanode-0")
      --uploadoss string      uploadoss flag，default is true (default "yes")
  -v, --verbose string        backup log level (default "0")

Use "iotdbbackuprestore [command] --help" for more information about a command.

```

### 命令行参数

命令行选项以及默认值

| 参数 | 描述                             | 默认值                  |
|:-------------------|--------------------------------|------------------------|
| `--namespace` | Kubernetes 命名空间名称              | `default`              |
| `--podName`        | Kubernetes Pod 名称              | `iotdb-datanode`  |
| `--dataDir`        | 容器中要备份的数据目录路径                  | `/data/iotdb`          |
| `--outname` | 备份文件的输出名称。 | `backup.tar.gz`        |
| `--config  `       | 指定 Kubernetes 配置文件路径。          | `~/.kube/config`        |
| `--bucketName`     | 阿里云 OSS 存储桶名称                  | `my-bucket`            |
| `-fileName`        | OSS 存储桶中的文件名称                  | `backup/backup.tar.gz` |
| `--kubeconfig`     | Kubernetes 配置文件路径              | `~/.kube/config`       |
| `--verbose`        | 日志输出详细级别（0、1、2) | `1`                    |
| `--keepLocal` | keepLocal 设置为 false（不保留本地文件） | `false` |
| `--chunkSize` | 指定分片下载、上传的大小 | `10MB` |
| `--uploadoss` |  |  |

### 命令行补全



```bash
# zsh
source <(iotdbtools completion zsh)
# bash
source <(iotdbtools completion bash)
```



### 示例

#### 备份 uat iotdb

```bash
iotdbtools backup --config /opt/kubeconfig/cce-ems-plusuat.kubeconfg  --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup  --outname emsuat --uploadoss=true --keep-local=true --datadir /iotdb/data/datanode --containers=iotdb-confignode --verbose=2
```



#### 备份 cn/eu iotdb

```bash
# CN 集群并行备份
iotdbtools backup --config /opt/kubeconfig/cce-ems-plusuat.kubeconfg  --namespace iotdb-bigdata  --pods=iotdb-datanode-0,iotdb-datanode-1,iotdb-datanode-2 --bucketname iotdb-backup  --outname emscn --uploadoss=true --keep-local=true --datadir /iotdb/data/datanode  --containers=iotdb-confignode --verbose=2

#EU
iotdbtools backup --namespace ems-eu --pods=iotdb-datanode-0 --bucketname="iotdb-backup" --outname=emseu  --uploadoss=true --keep-local=true --containers=iotdb-confignode --verbose=2
```



#### 备份其他 pod 的指定文件

备份指定 pod、container 中指定目录到本地或 oss，使用场景: 定时任务上传pod中的文件

- 配置中心
- coredump文件定时上传

```bash
 iotdbtools backup --config /opt/kubeconfig/eks-config-ems-eu-newaccount --namespace ems-eu --pods vnnox-middle-configcenter-7459fcfb5b-6x8gz --datadir /tmp --containers vnnox-middle-configcenter --uploadoss true --bucketname iotdb-backup --keep-local false  --verbose 2
```

#### 恢复 cn 的备份

```bash
iotdbtools restore --config .config --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup --verbose 2 --file emseu-workstaaa_iotdb-datanode-0_iotdb-datanode_20240822094200.tar.gz
```

### 配置

默认将备份文件上传到 oss，可以通过 uploadoss 关闭

OSS 的访问凭证保存到本地的 .credentials 文件中，请妥善保存

```b
AK=your-access-key
SK=your-secret-key
ENDPOINT=your-oss-endpoint
```

### 日志输出

日志详细级别可以通过 --verbose 标志来设置。
日志级别 0 将不输出任何日志，适合静默执行。
日志级别 1 将输出基本操作日志。
日志级别 2 将输出详细日志，适合调试和问题排查。
### 其他
- 统计代码行数

```bash
`find . -name "*.go"  | xargs -I GG echo "wc -l GG" | bash | awk '{sum+=$1} END {print sum}'
1725
```

# 7. Release

## 2.0

```bash
1. 新增恢复iotdb备份逻辑
```



## 2.1

```bash
1. 新增keep-local参数
2. 新增uploadoss 参数控制上传逻辑
keep-local为true时备份先落到从本地上传到OSS，使用oss-go-sdk，否则使用ossutil上传.
```

## 2.2

```bash
1. 增加企业微信通知
2. 新增分片上传参数chunksize
3. 增加集群备份并行逻辑
```

## 2.3 

```bash
1. 新增hook判断逻辑，提升适配性。可备份任何pod中的文件
2. fix issue: 失败时也发送成功通知的
3. 增加性能记录函数trackStepDuration
4. 适配多级bucketname
5. 新增iotdbtools 命令行补全
```

