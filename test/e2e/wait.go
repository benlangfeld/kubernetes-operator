package e2e

import (
	goctx "context"
	"net/http"
	"testing"
	"time"

	"github.com/jenkinsci/kubernetes-operator/internal/try"
	"github.com/jenkinsci/kubernetes-operator/pkg/apis/jenkins/v1alpha2"
	jenkinsclient "github.com/jenkinsci/kubernetes-operator/pkg/controller/jenkins/client"
	"github.com/jenkinsci/kubernetes-operator/pkg/controller/jenkins/configuration/base/resources"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	retryInterval = time.Second * 5
	timeout       = time.Second * 60
)

// checkConditionFunc is used to check if a condition for the jenkins CR is set
type checkConditionFunc func(*v1alpha2.Jenkins, error) bool

func waitForJenkinsBaseConfigurationToComplete(t *testing.T, jenkins *v1alpha2.Jenkins) {
	t.Log("Waiting for Jenkins base configuration to complete")
	_, err := WaitUntilJenkinsConditionSet(retryInterval, 150, jenkins, func(jenkins *v1alpha2.Jenkins, err error) bool {
		t.Logf("Current Jenkins status: '%+v', error '%s'", jenkins.Status, err)
		return err == nil && jenkins.Status.BaseConfigurationCompletedTime != nil
	})
	assert.NoError(t, err)
	t.Log("Jenkins pod is running")

	// update jenkins CR because Operator sets default values
	namespacedName := types.NamespacedName{Namespace: jenkins.Namespace, Name: jenkins.Name}
	err = framework.Global.Client.Get(goctx.TODO(), namespacedName, jenkins)
	assert.NoError(t, err)
}

func waitForRecreateJenkinsMasterPod(t *testing.T, jenkins *v1alpha2.Jenkins) {
	err := wait.Poll(retryInterval, 30*retryInterval, func() (bool, error) {
		lo := metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(resources.BuildResourceLabels(jenkins)).String(),
		}
		podList, err := framework.Global.KubeClient.CoreV1().Pods(jenkins.ObjectMeta.Namespace).List(lo)
		if err != nil {
			return false, err
		}
		if len(podList.Items) != 1 {
			return false, nil
		}

		return podList.Items[0].DeletionTimestamp == nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Jenkins pod has been recreated")
}

func waitForJenkinsUserConfigurationToComplete(t *testing.T, jenkins *v1alpha2.Jenkins) {
	t.Log("Waiting for Jenkins user configuration to complete")
	_, err := WaitUntilJenkinsConditionSet(retryInterval, 70, jenkins, func(jenkins *v1alpha2.Jenkins, err error) bool {
		t.Logf("Current Jenkins status: '%+v', error '%s'", jenkins.Status, err)
		return err == nil && jenkins.Status.UserConfigurationCompletedTime != nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Jenkins pod is running")
}

func waitForJenkinsSafeRestart(t *testing.T, jenkinsClient jenkinsclient.Jenkins) {
	err := try.Until(func() (end bool, err error) {
		status, err := jenkinsClient.Poll()
		if err != nil {
			return false, err
		}
		if status != http.StatusOK {
			return false, errors.Wrap(err, "couldn't poll data from Jenkins API")
		}
		return true, nil
	}, time.Second, time.Second*70)
	require.NoError(t, err)
}

// WaitUntilJenkinsConditionSet retries until the specified condition check becomes true for the jenkins CR
func WaitUntilJenkinsConditionSet(retryInterval time.Duration, retries int, jenkins *v1alpha2.Jenkins, checkCondition checkConditionFunc) (*v1alpha2.Jenkins, error) {
	jenkinsStatus := &v1alpha2.Jenkins{}
	err := wait.Poll(retryInterval, time.Duration(retries)*retryInterval, func() (bool, error) {
		namespacedName := types.NamespacedName{Namespace: jenkins.Namespace, Name: jenkins.Name}
		err := framework.Global.Client.Get(goctx.TODO(), namespacedName, jenkinsStatus)
		return checkCondition(jenkinsStatus, err), nil
	})
	if err != nil {
		return nil, err
	}
	return jenkinsStatus, nil
}
