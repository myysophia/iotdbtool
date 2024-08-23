# IoTDBBackup
## 项目简介
iotdbbackup 是一个使用 Go 语言编写的命令行工具，基于 Kubernetes 环境，提供了 IoTDB 数据的备份功能。它可以从 Kubernetes 集群中的 IoTDB Pod 中提取数据，并将其上传到阿里云 OSS 存储桶中。

## 功能特性
从 Kubernetes 集群中的 IoTDB Pod 中提取数据。
支持Pod 中的指定目录。
将备份文件上传到阿里云 OSS。
支持多种日志输出级别，便于调试和监控。

## 系统要求
Go 1.16 或更高版本
Kubernetes 集群
阿里云 OSS 存储桶和相应的访问权限
## 安装
### 从源码构建
首先，确保你已经安装了 Go 语言开发环境。然后，克隆项目并编译二进制文件：

```bash
git clone iotdbbackup.git
cd iotdbbackup
go build -o iotdbbackup
```

### 交叉编译（在 Windows 上编译 Linux 二进制文件）
```bash
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -o iotdbbackup


# linux 上build 语法
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o iotdbbackupv4
```

 设置为 0，可以避免对系统上 C 库的依赖，从而生成更加通用的二进制文件。

编译完成后，你将获得一个 iotdbbackup 二进制文件。

## 使用指南
### 基本用法
```bash
iotdbbackuprestore is a CLI tool to backup and restore IoTDB data in Kubernetes.

Usage:
  iotdbbackuprestore [command]

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



### 示例

#### 备份uat iotdb

```bash
./iotdbbackuprestorev2 backup --config /root/.kube/config  --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup --datadir /iotdb/data/ --verbose 1 --outname emsuat
```



#### 备份 cn iotdb

```bash
./iotdbbackuprestorev2 backup --config /root/.kube/config  --namespace ems-plus-mapai --pods=iotdb-datanode-0,iotdb-datanode-1,iotdb-datanode-2 --bucketname iotdb-backup --datadir /iotdb/data/ --cluster-name emscn --uploadoss true
```



#### 备份其他pod 的指定文件

备份指定pod、container中指定目录到本地或oss

```bash
iotdbbackuprestorev2 backup --namespace ems-eu --pods vnnox-middle-configcenter-7459fcfb5b-6x8gz --datadir /tmp --containers vnnox-middle-configcenter --uploadoss true --bucketname iotdb-backup --keep-local false  --verbose 2
```

#### 恢复cn 的备份

```bash
iotdbbackuprestorev2 restore --config .config --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup --verbose 2 --file emseu-workstaaa_iotdb-datanode-0_iotdb-datanode_20240822094200.tar.gz
```

#### 数据比对

```bash
show devices 比较设备数量
count timeseries 比较时间序列总数
```

### 配置

默认将备份文件上传到oss，可以通过uploadoss关闭

OSS 的访问凭证保存到 .credentials 文件中，请妥善保存

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
