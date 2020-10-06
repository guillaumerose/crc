package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	certificatesclient "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/util/certificate"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	kubeletcertificate "k8s.io/kubernetes/pkg/kubelet/certificate"
	"k8s.io/kubernetes/pkg/kubelet/certificate/bootstrap"
)

type rotater interface {
	RotateCerts() (bool, error)
}

type options struct {
	KubeConfig          string
	BootstrapKubeconfig string
	CertDirectory       string
	NodeName            string
}

func main() {
	var s options
	flag.StringVar(&s.NodeName, "node-name", "", "")
	flag.StringVar(&s.KubeConfig, "kubeconfig", "", "")
	flag.StringVar(&s.BootstrapKubeconfig, "bootstrap-kubeconfig", "", "")
	flag.StringVar(&s.CertDirectory, "cert-dir", "", "")
	flag.Parse()

	certConfig, clientConfig, err := bootstrap.LoadClientConfig(s.KubeConfig, s.BootstrapKubeconfig, s.CertDirectory)
	if err != nil {
		log.Fatal(err)
	}
	m, err := buildClientCertificateManager(certConfig, clientConfig, s.CertDirectory, types.NodeName(s.NodeName))
	if err != nil {
		log.Fatal(err)
	}
	r := m.(rotater)
	done, err := r.RotateCerts()
	if err != nil {
		log.Fatal(err)
	}
	if done {
		fmt.Println("Client certs rotated")
	} else {
		fmt.Println("Client certs NOT rotated")
	}

	m, err = buildServerCertificateManager(clientConfig, s.CertDirectory)
	if err != nil {
		log.Fatal(err)
	}
	r = m.(rotater)
	done, err = r.RotateCerts()
	if err != nil {
		log.Fatal(err)
	}
	if done {
		fmt.Println("Serving certs rotated")
	} else {
		fmt.Println("Serving certs NOT rotated")
	}
}

func buildClientCertificateManager(certConfig, clientConfig *restclient.Config, certDir string, nodeName types.NodeName) (certificate.Manager, error) {
	newClientsetFn := func(current *tls.Certificate) (certificatesclient.CertificateSigningRequestInterface, error) {
		// If we have a valid certificate, use that to fetch CSRs. Otherwise use the bootstrap
		// credentials. In the future it would be desirable to change the behavior of bootstrap
		// to always fall back to the external bootstrap credentials when such credentials are
		// provided by a fundamental trust system like cloud VM identity or an HSM module.
		config := certConfig
		if current != nil {
			config = clientConfig
		}
		c, err := clientset.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		return c.CertificatesV1beta1().CertificateSigningRequests(), nil
	}

	return kubeletcertificate.NewKubeletClientCertificateManager(
		certDir,
		nodeName,

		// this preserves backwards compatibility with kubeadm which passes
		// a high powered certificate to the kubelet as --kubeconfig and expects
		// it to be rotated out immediately
		clientConfig.CertData,
		clientConfig.KeyData,

		clientConfig.CertFile,
		clientConfig.KeyFile,
		newClientsetFn,
	)
}

func buildServerCertificateManager(clientConfig *restclient.Config, certDir string) (certificate.Manager, error) {
	c, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return kubeletcertificate.NewKubeletServerCertificateManager(c, &kubeletconfig.KubeletConfiguration{
		TLSCertFile:       "",
		TLSPrivateKeyFile: "",
	}, types.NodeName(list.Items[0].Name), func() []v1.NodeAddress {
		return list.Items[0].Status.Addresses
	}, certDir)
}
