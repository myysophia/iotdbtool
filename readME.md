# iotdbtool
## 项目简介
iotdbtool是一个使用 Go 语言编写的命令行工具，基于 Kubernetes 环境，提供了 IoTDB 数据的备份功能。它可以从 Kubernetes 集群中的 IoTDB Pod 中提取数据，并将其上传到阿里云 OSS 存储桶中。

iotdbtool支持iotDB单机、集群，备份与恢复，备份文件存储在oss上，主要实现了k8s部署的有状态服务的备份恢复

## 痛点

- 开源版本iotdb 没有现成的冷备方案，exporttsfile基本不可用，引用几十个jar导出全库需要巨量内存。
- 业务核心组件，iotdb down = 业务不可用down。 每天2次冷备，以备不时之需
- 多个iotdb 节点，备份恢复没有一个all in one 简单的工具

## 功能特性

- 支持任意Pod任意容器中的指定目录。
- 将备份文件上传到阿里云 OSS，默认保存7天
- 支持多种日志输出级别，便于调试和监控。
- 支持iotdb单机、集群的备份和恢复
- goroutine 支持并发备份
- oss sdk 上传oss、分片上传、进度条
- 不依赖kubectl命令，使用client-go 直接调用api操作pod，安全高效

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
go build -o iotdbbackuprestore
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

编译完成后，你将获得一个 iotdbbackup 二进制文件。可以到处运行

## 使用指南
### backup
![backup](https://github.com/user-attachments/assets/e24fce0a-b7b9-422b-a5b2-5b1aa5add0ef)

### restore 

```bash
./iotdbbackuprestorev2 restore --config /opt/kubeconfig/eks-config-ems-eu-newaccount  --namespace iotdbtest --pods=iotdb-datanode-0 --bucketname iotdb-backup --verbose 1 --file emsdev_iotdb-datanode-0_20240830152642.tar.gz
```



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
iotdbtools backup --config /opt/kubeconfig/cce-ems-plusuat.kubeconfg  --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup  --outname emsuat --uploadoss=true --keep-local=true --datadir /iotdb/data/datanode --containers=iotdb-confignode --verbose=2
开始时间: 2024-09-05 15:32:53
正在处理 pod: iotdb-datanode-0
正在处理容器: iotdb-confignode
Flush command output: Msg: The statement is executed successfully.

刷新数据 完成，耗时: 7.206705227s
压缩文件 emsuat_iotdb-datanode-0_20240905153253.tar.gz 成功
压缩数据 完成，耗时: 2m4.26350324s
文件 emsuat_iotdb-datanode-0_20240905153253.tar.gz 已从 pod iotdb-datanode-0 复制到本地
复制备份文件到本地 完成，耗时: 33.381891837s
emsuat_iotdb-datanode-0_20240905153253.tar.gz 正在上传 100% |██████████████████████████████████████████████████████████████████████████████████████████████████████| (1.4/1.4 GB, 27 MB/s)
文件 emsuat_iotdb-datanode-0_20240905153253.tar.gz 已上传到OSS，将在 2024-09-12 15:35:38 后自动删除
从本地上传到OSS 完成，耗时: 51.59761308s
pod iotdb-datanode-0 的备份完成。耗时: 3m36.449795156s
已发送企业微信通知
结束时间: 2024-09-05 15:36:30
总耗时: 3m36.691697783s

```



#### 备份 cn/eu iotdb

```bash
# CN 集群并行备份
iotdbtools backup --config /opt/kubeconfig/cce-ems-plusuat.kubeconfg  --namespace iotdb-bigdata  --pods=iotdb-datanode-0,iotdb-datanode-1,iotdb-datanode-2 --bucketname iotdb-backup  --outname emscn --uploadoss=true --keep-local=true --datadir /iotdb/data/datanode  --containers=iotdb-confignode --verbose=2
开始时间: 2024-09-05 15:50:31
正在处理 pod: iotdb-datanode-2
正在处理 pod: iotdb-datanode-1
正在处理容器: iotdb-confignode
正在处理容器: iotdb-confignode
正在处理 pod: iotdb-datanode-0
正在处理容器: iotdb-confignode
Flush command output: Msg: The statement is executed successfully.

Flush command output: Msg: The statement is executed successfully.

Flush command output: Msg: The statement is executed successfully.

刷新数据 完成，耗时: 5.909704181s
刷新数据 完成，耗时: 5.925296428s
刷新数据 完成，耗时: 6.148190492s
压缩文件 emscn_iotdb-datanode-2_20240905155031.tar.gz 成功
压缩数据 完成，耗时: 49.883324209s
压缩文件 emscn_iotdb-datanode-0_20240905155031.tar.gz 成功
压缩数据 完成，耗时: 1m8.42604954s
文件 emscn_iotdb-datanode-2_20240905155031.tar.gz 已从 pod iotdb-datanode-2 复制到本地
复制备份文件到本地 完成，耗时: 22.414878967s
emscn_iotdb-datanode-2_20240905155031.tar.gz 正在上传  53% |██████████████████████████████████████████████████████                                                | (493/925 MB, 28 MB/s) [16s:15s]压缩文件 emscn_iotdb-datanode-1_20240905155031.tar.gz 成功
压缩数据 完成，耗时: 1m29.660285987s
emscn_iotdb-datanode-2_20240905155031.tar.gz 正在上传 100% |███████████████████████████████████████████████████████████████████████████████████████████████████████| (925/925 MB, 27 MB/s)
文件 emscn_iotdb-datanode-2_20240905155031.tar.gz 已上传到OSS，将在 2024-09-12 15:51:49 后自动删除
从本地上传到OSS 完成，耗时: 34.027278099s
pod iotdb-datanode-2 的备份完成。耗时: 1m52.250870027s
已发送企业微信通知
文件 emscn_iotdb-datanode-1_20240905155031.tar.gz 已从 pod iotdb-datanode-1 复制到本地
复制备份文件到本地 完成，耗时: 31.440093628s
emscn_iotdb-datanode-1_20240905155031.tar.gz 正在上传  13% |█████████████                                                                                       | (157 MB/1.2 GB, 39 MB/s) [3s:25s]文件 emscn_iotdb-datanode-0_20240905155031.tar.gz 已从 pod iotdb-datanode-0 复制到本地
复制备份文件到本地 完成，耗时: 56.685553013s
emscn_iotdb-datanode-0_20240905155031.tar.gz 正在上传 100% |███████████████████████████████████████████████████████████████████████████████████████████████████████| (1.2/1.2 GB, 23 MB/s)
文件 emscn_iotdb-datanode-0_20240905155031.tar.gz 已上传到OSS，将在 2024-09-12 15:52:42 后自动删除
从本地上传到OSS 完成，耗时: 53.647831764s
pod iotdb-datanode-0 的备份完成。耗时: 3m4.907742483s
已发送企业微信通知
emscn_iotdb-datanode-1_20240905155031.tar.gz 正在上传 100% |█████████████████████████████████████████████████████████████████████████████████████████████████████| (1.2/1.2 GB, 13 MB/s)
文件 emscn_iotdb-datanode-1_20240905155031.tar.gz 已上传到OSS，将在 2024-09-12 15:52:38 后自动删除
从本地上传到OSS 完成，耗时: 1m32.410085684s
pod iotdb-datanode-1 的备份完成。耗时: 3m39.420246496s
已发送企业微信通知
结束时间: 2024-09-05 15:54:10
总耗时: 3m39.614650646s


#EU
iotdbtools backup --namespace ems-eu --pods=iotdb-datanode-0 --bucketname="iotdb-backup" --outname=emseu  --uploadoss=true --keep-local=true --containers=iotdb-confignode --verbose=2
开始时间: 2024-09-05 07:41:23
正在处理 pod: iotdb-datanode-0
正在处理容器: iotdb-confignode
Flush command output: Msg: The statement is executed successfully.

刷新数据 完成，耗时: 6.527718513s
压缩文件 emseu_iotdb-datanode-0_20240905074123.tar.gz 成功
压缩数据 完成，耗时: 5.380294878s
文件 emseu_iotdb-datanode-0_20240905074123.tar.gz 已从 pod iotdb-datanode-0 复制到本地
复制备份文件到本地 完成，耗时: 2.018793187s
emseu_iotdb-datanode-0_20240905074123.tar.gz 正在上传 100% |███████████████████████████████████████████████████████████████████████████████████████████████████████| (163/163 MB, 16 MB/s)
文件 emseu_iotdb-datanode-0_20240905074123.tar.gz 已上传到OSS，将在 2024-09-12 07:41:37 后自动删除
从本地上传到OSS 完成，耗时: 10.79072678s
pod iotdb-datanode-0 的备份完成。耗时: 24.718321162s
已发送企业微信通知
结束时间: 2024-09-05 07:41:49
总耗时: 26.303233462s
```



#### 备份其他pod 的指定文件

备份指定pod、container中指定目录到本地或oss

```bash
iotdbtools backup --namespace ems-eu --pods vnnox-middle-configcenter-7459fcfb5b-6x8gz --datadir /tmp --containers vnnox-middle-configcenter --uploadoss true --bucketname iotdb-backup --keep-local false  --verbose 2
```

#### 恢复cn 的备份

```bash
iotdbbackuprestorev2 restore --config .config --namespace ems-uat --pods=iotdb-datanode-0 --bucketname iotdb-backup --verbose 2 --file emseu-workstaaa_iotdb-datanode-0_iotdb-datanode_20240822094200.tar.gz
```

### 配置

默认将备份文件上传到oss，可以通过uploadoss关闭

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

