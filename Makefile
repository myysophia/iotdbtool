
bin/wechat-webhook:
	 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags "-w" -o bin/wechat-webhook

# make tke version=wechat-webhook
tke: bin/wechat-webhook
	docker build -t  registry.cn-hangzhou.aliyuncs.com/novacloud/wechat-webhook-new:ems1.1 . \
    docker push registry.cn-hangzhou.aliyuncs.com/novacloud/wechat-webhook-new:ems1.1

	