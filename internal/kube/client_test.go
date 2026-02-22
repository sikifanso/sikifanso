package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClientForCluster_NoKubeconfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")

	_, err := ClientForCluster("nonexistent")
	if err == nil {
		t.Fatal("expected error when kubeconfig does not exist, got nil")
	}
}

func TestNodeReady_True(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	if !NodeReady(node) {
		t.Error("expected NodeReady to return true")
	}
}

func TestNodeReady_False(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}
	if NodeReady(node) {
		t.Error("expected NodeReady to return false")
	}
}

func TestNodeReady_NoCondition(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}
	if NodeReady(node) {
		t.Error("expected NodeReady to return false when no conditions present")
	}
}
