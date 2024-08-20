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
```

 设置为 0，可以避免对系统上 C 库的依赖，从而生成更加通用的二进制文件。

编译完成后，你将获得一个 iotdbbackup 二进制文件。

## 使用指南
### 基本用法
```bash
./iotdbbackup backup [flags]
```



## 命令行参数

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



### 示例

备份指定的 Pod 并上传到 OSS
复制代码
./iotdbbackup backup --config /path/to/kubeconfig --namespace default --pods=iotdb-datanode-0 --bucketname my-bucket --datadir /iotdb/data/ --verbose 2
使用标签选择器备份 Pod 并上传到 OSS
bash
复制代码
./iotdbbackup backup --config /path/to/kubeconfig --namespace default --label app=iotdb --bucketname my-bucket --verbose 1
通过 Kubernetes 配置文件备份并上传到 OSS

./iotdbbackup backup --config /home/user/.kube/config --namespace my-namespace --pods=iotdb-datanode-0,iotdb-datanode-1 --bucketname my-bucket --outname backup.tar.gz --verbose 2

#### 配置文件

##### 阿里云 OSS 配置

将阿里云 OSS 的访问凭证保存到 .credentials 文件中，格式如下：
AK=your-access-key
SK=your-secret-key
ENDPOINT=your-oss-endpoint
将该文件放置在项目根目录下，或者你可以修改 loadCredentials 函数以读取其他位置的凭证文件。

日志输出
日志详细级别可以通过 --verbose 标志来设置。
日志级别 0 将不输出任何日志，适合静默执行。
日志级别 1 将输出基本操作日志。
日志级别 2 将输出详细日志，适合调试和问题排查。
贡献
我们欢迎社区的贡献！如果你想为 iotdbbackup 做出贡献，请遵循以下步骤：

Fork 此仓库。
创建你的功能分支 (git checkout -b feature/YourFeature)。
提交你的更改 (git commit -m 'Add YourFeature')。
推送到分支 (git push origin feature/YourFeature)。
创建一个新的 Pull Request。