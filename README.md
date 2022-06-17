# mysql-mgr

安装go：

```
yum -y install go
go env -w GOPATH="/data/go"
mkdir /data/go
go env -w GOPROXY=https://goproxy.cn
```

安装kubebuilder:
```
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.2.0/kubebuilder_linux_amd64
mv kubebuilder_linux_amd64 /usr/local/bin/kubebuilder
```

初始化项目：
```
mkdir /data/go/mysql
kubebuilder init --domain emergen.cn --repo mysqlmgr
kubebuilder create api --group publicapp --version v1 --kind Mysql
make manifests
go mod tidy
make install
make run
```

打包代码并应用至k8s：
```
#修改Dockerfile里的镜像
vim Dockerfile
    kubeimages/distroless-static:latest
#修改manager_auth_proxy_patch.yaml镜像
vim config/default/manager_auth_proxy_patch.yaml
    kubesphere/kube-rbac-proxy:v0.8.0

make docker-build docker-push IMG=172.31.0.5:5000/k8s/mysql-kubebuilder:v1.0
make deploy IMG=172.31.0.5:5000/k8s/mysql-kubebuilder:v1.0
#应用cr
kubectl apply -f config/samples/publicapp_v1_mysql.yaml 
```
