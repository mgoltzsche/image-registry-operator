package merge

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func AddPort(svc *corev1.Service, name string, port, targetPort int32, prot corev1.Protocol) {
	for _, p := range svc.Spec.Ports {
		if p.Name == name && p.Port == port && p.TargetPort.IntVal == targetPort && p.Protocol == prot {
			return // port already exists
		}
	}
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       name,
			Port:       port,
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: targetPort},
			Protocol:   prot,
		},
	}
}
