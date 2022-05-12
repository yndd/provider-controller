module github.com/yndd/provider-controller

go 1.17

require (
	github.com/cert-manager/cert-manager v1.8.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/pkg/errors v0.9.1
	github.com/yndd/ndd-core v0.2.14
	github.com/yndd/ndd-runtime v0.5.8
	github.com/yndd/ndd-target-runtime v0.0.40
	github.com/yndd/registrator v0.0.4
	k8s.io/api v0.24.0
	k8s.io/apiextensions-apiserver v0.24.0
	k8s.io/apimachinery v0.24.0
	k8s.io/client-go v0.24.0
	sigs.k8s.io/controller-runtime v0.12.0
)

require cloud.google.com/go/storage v1.18.2 // indirect
