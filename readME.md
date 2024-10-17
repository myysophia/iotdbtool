# iotdbtool
## 项目简介
iotdbtool 是一个使用 Go 语言编写的命令行工具，基于 Kubernetes 环境，提供了 IoTDB 数据的备份功能。它可以从 Kubernetes 集群中的 IoTDB Pod 中提取数据，并将其上传到阿里云 OSS 存储桶中。

iotdbtool 支持 iotDB 单机、集群，备份与恢复，备份文件存储在 oss 上，主要实现了 k8s 部署的有状态服务的备份恢复

### croba

`cobra`，它是一个非常流行的 Go 库，主要用于创建命令行程序（CLI）。它提供了强大的命令行解析功能，帮助开发者快速构建功能丰富、结构化的命令行工具。

常见使用场景：

- 开发复杂的命令行工具和应用程序。
- 结合 `viper` 用于解析配置文件。
- `kubectl` 等工具就是基于 `cobra` 开发的。



## 痛点

- 备份步骤相当繁琐，比较耗时，备份业务全部环境大约1-2H

1. 准备kubeconfig文件连接k8s集群
2. 使用kubectl命令行进入iotdb pod ，使用iotdb客户端连接iotdb
3. 在iotdb数据库中执行flush 保证memory table中的数据落盘
4. 压缩iotdb 的数据目录
5. 下载ossutil、配置AK/SK。
6. 等待步骤4执行完后执行ossutil cp命令将备份上传到oss上
7. 循环步骤2-6备份iotdb其他数据节点
8. 重复步骤1-6备份备份其他iotdb集群
9. 备份完成

- 严重依赖kubectl、iotdb-cli、ossutil等其他命令

## 功能特性

- 支持 iotdb 单机、集群的备份和恢复
- goroutine 原生支持并发备份
- 借助ali-oss-sdk 实现分片上传、进度条等功能
- 不依赖 kubectl 命令，使用 client-go 直接调用 api 操作 pod，安全高效
- 支持任意 Pod 任意容器中的指定目录
- 支持 prehook，备份前flush on cluster强刷盘
- 将备份文件上传到阿里云 OSS，默认保存 3 天
- 支持多种日志输出级别，便于调试和监控。

## 系统要求

go mod依赖

```
aliyun-oss-go-sdk v3.0.2+incompatible

github.com/spf13/cobra v1.7.0

k8s.io/api v0.27.3

k8s.io/client-go v0.27.3
```

阿里云oss 上传文件权限 <可选>

k8s集群访问权限

## 安装

### 从源码构建
首先，确保你已经安装了 Go 1.21 开发环境。然后，克隆项目并编译二进制文件：

```bash
.
|-- Makefile
|-- bin
|   `-- iotdbtools
|-- cmd
|   |-- backup.go
|   |-- oss_config.go
|   |-- restore.go
|   `-- root.go
|-- command.log
|-- go.mod
|-- go.sum
|-- iotdbtool-architecture.svg
|-- main.go
`-- readME.md

git clone http://git/iotdbtool.git
cd iotdbtool
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags "-w" -o bin/iotdbtools

```

**`-a`**:

- **作用**：强制重新编译所有依赖包，不论这些包是否已经被编译过或者是否存在缓存。Go 编译器会缓存依赖包以提高编译速度，默认情况下只重新编译必要的包。加上 `-a` 标志后，所有依赖包都会重新编译，适用于在编译时需要避免使用缓存的场景。

**`-installsuffix cgo`**:

- **作用**：为编译过程生成的目标文件（object files）加上一个后缀（这里是 `cgo`）。
- `-ldflags` 传递链接器的选项给 Go 编译器的链接器。。`-w`：此标志告诉 Go 编译器的链接器不要生成调试信息（例如符号表、调试符号等），这样可以**减小二进制文件的大小**

### 直接下载二进制

```bash
wget https://nova-software-download.oss-cn-hangzhou.aliyuncs.com/iotdbtools && chmod +x iotdbtools && mv iotdbtools /usr/local/bin
```



## 使用指南
### backup
```bash
 iotdbtools backup --config /opt/kubeconfig/cce-yx --namespace ems-uat --pods iotdb-datanode-0 --bucketname iotdb-backup \
--outname emsyx --uploadoss true --keep-local true --verbose 2
```
![backup](https://github.com/user-attachments/assets/6e1188e0-1c0f-4a72-9d47-273d3dc74236)


### restore 

```bash
 iotdbtools restore --config /opt/kubeconfig/eks-config-ems-eu-newaccount --namespace iotdb --pods iotdb-datanode-0 \
--bucketname iotdb-backup --file emsyx_iotdb-datanode-0_20240906154128.tar.gz --verbose 1
```
![restore](https://github.com/user-attachments/assets/ddb7f952-f204-445f-88a6-9da06c818392)



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

Use "iotdbtools [command] --help" for more information about a command.

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

corba

```bash
# zsh
source <(iotdbtools completion zsh)
# bash
source <(iotdbtools completion bash)
```



### 示例

#### 备份  iotdb

```bash
iotdbtools backup --config /opt/kubeconfig/cce-ems-plusuat.kubeconfg  --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup  --outname emsuat --uploadoss=true --keep-local=true --datadir /iotdb/data/datanode --containers=iotdb-confignode --verbose=2
```




#### 备份其他 pod 的指定文件

备份指定 pod、container 中指定目录到本地或 oss，使用场景: 定时任务上传pod中的文件

- 配置中心
- coredump文件定时上传

```bash
 iotdbtools backup --config /opt/kubeconfig/eks-config-ems-eu-newaccount --namespace ems-eu --pods vnnox-middle-configcenter-7459fcfb5b-6x8gz --datadir /tmp --containers vnnox-middle-configcenter --uploadoss true --bucketname iotdb-backup --keep-local false  --verbose 2
```

#### 恢复备份

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
- 企微通知

替换为自己的key即可发送企微通知

# 7. Release Note

## 2.3 

```bash
1. 新增hook判断逻辑，提升适配性。可备份任何pod中的文件
2. fix issue: 失败时也发送成功通知的
3. 增加性能记录函数trackStepDuration
4. 适配多级bucketname
5. 新增iotdbtools 命令行补全功能
```

## 2.2

```bash
1. 增加企业微信通知
2. 新增分片上传参数chunksize
3. 增加集群备份并行逻辑
```

## 2.1

```bash
1. 新增keep-local参数
2. 新增uploadoss 参数控制上传逻辑
keep-local为true时备份先落到从本地上传到OSS，使用oss-go-sdk，否则使用ossutil上传.

```

## 2.0

```bash
1. 新增恢复iotdb备份逻辑
```



# 展望与鸣谢

这个工具刚开始叫iotdbbackuprestore 后来改成了iotdbtool。  有些运维操作都可以集成进来，例如iotdb部署、迁移(单机到集群、集群到单机、跨云/IDC迁移)、数据比对....

例如

- iotdbtool create cluster 1c3d.yaml 通过声明式的方式创建集群，背后由iotdb operator驱动
-  iotdbtool data diff   数据比较

 特别感谢chatgpt 和通义千问
