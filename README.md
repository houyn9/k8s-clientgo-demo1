# k8scontroller
## 通过对deployment的annotations配置，实现自动创建svc和ingress，监测svc和ingress的稳定状态。

1.go mod init clientgo01

2.go get k8s.io/client-go@v0.21.5

3.go mod tidy
