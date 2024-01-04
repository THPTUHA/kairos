package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

type K8sDeploy struct {
}

func (k *K8sDeploy) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {

	out, err := k.ExecuteImpl(args)
	resp := &proto.ExecuteResponse{
		Output: out,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, err
}

func (s *K8sDeploy) ExecuteImpl(args *proto.ExecuteRequest) ([]byte, error) {
	home := homedir.HomeDir()
	kubeconfig := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	cmd := args.Config["cmd"]
	if cmd == "" {
		return nil, fmt.Errorf("empty cmd")
	}

	serviceName := args.Config["service"]
	namespace := args.Config["namespace"]
	if namespace == "" {
		namespace = "default"
	}
	switch cmd {
	case "deploy":
		return s.deploy(clientset, namespace, serviceName)
		// return []byte("success"), nil
	case "log":
		return s.log(clientset, namespace, &PodQuery{
			Name: serviceName,
		})
	default:
		return nil, fmt.Errorf("invalid cmd")
	}
}

func (s *K8sDeploy) deploy(clientset *kubernetes.Clientset, namespace, serviceName string) ([]byte, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("empty service name")
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "RESTARTED_AT",
				Value: time.Now().Format(time.RFC3339),
			},
		)

		_, err = clientset.AppsV1().Deployments(namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
		return err
	})

	if err != nil {
		return nil, err
	}

	err = s.waitForPodReady(clientset, namespace, serviceName)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("restart deployment %s namespace %s success", serviceName, namespace)), nil
}

type PodStatus struct {
	Name              string                   `json:"name"`
	Status            string                   `json:"status"`
	StartTime         *metav1.Time             `json:"start_time"`
	ContainerStatuses []corev1.ContainerStatus `json:"container_statues"`
}

type PodQuery struct {
	Name string `json:"name"`
}

func (s *K8sDeploy) log(clientset *kubernetes.Clientset, namespace string, podName *PodQuery) ([]byte, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ps := make([]*PodStatus, 0)

	for _, pod := range pods.Items {
		if podName.Name != "" {
			if strings.HasPrefix(pod.GetName(), podName.Name) {
				ps = append(ps, &PodStatus{
					Name:              pod.GetName(),
					Status:            string(pod.Status.Phase),
					StartTime:         pod.Status.StartTime,
					ContainerStatuses: pod.Status.ContainerStatuses,
				})
			}
		} else {
			ps = append(ps, &PodStatus{
				Name:              pod.GetName(),
				Status:            string(pod.Status.Phase),
				StartTime:         pod.Status.StartTime,
				ContainerStatuses: pod.Status.ContainerStatuses,
			})
		}
	}

	pj, err := json.Marshal(ps)
	if err != nil {
		return nil, err
	}
	return pj, nil
}

func (s *K8sDeploy) waitForPodReady(clientset *kubernetes.Clientset, namespace, deploymentName string) error {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)

	return wait.PollImmediate(time.Second, time.Minute*2, func() (bool, error) {
		deployment, err := deploymentsClient.Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		conditions := deployment.Status.Conditions
		for _, condition := range conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == "True" {
				return true, nil
			}
		}

		return false, nil
	})
}
