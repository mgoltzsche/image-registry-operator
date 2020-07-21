package merge

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func AddServicePort(svc *corev1.Service, name string, port, targetPort int32, prot corev1.Protocol) {
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

func AddContainer(pod *corev1.PodSpec, c corev1.Container) {
	for i, existing := range pod.Containers {
		if existing.Name == c.Name {
			pod.Containers[i] = c
			return
		}
	}
	pod.Containers = append(pod.Containers, c)
}

func AddVolume(pod *corev1.PodSpec, v corev1.Volume) {
	for i, existing := range pod.Volumes {
		if existing.Name == v.Name {
			pod.Volumes[i] = v
			return
		}
	}
	pod.Volumes = append(pod.Volumes, v)
}

func DelVolume(pod *corev1.PodSpec, volName string) {
	for i, existing := range pod.Volumes {
		if existing.Name == volName {
			pod.Volumes = append(pod.Volumes[:i], pod.Volumes[i+1:]...)
			return
		}
	}
}
