package main

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
)

type DeploymentMonitoringController struct {
	informerFactory    informers.SharedInformerFactory
	deploymentInformer appsinformers.DeploymentInformer
	clientset          kubernetes.Clientset
}

func (c *DeploymentMonitoringController) updateDeployment(deploy *appsv1.Deployment) {
	image := deploy.Spec.Template.Spec.Containers[0].Image
	tag := image[strings.LastIndex(image, ":")+1:]
	savedTag := deploy.Annotations["atomicjolt.com/release-notifier-saved-tag"]
	fmt.Printf("Checking deployment %s/%s, Image: %s, Saved Tag: %s\n", deploy.Namespace, deploy.Name, tag, savedTag)
	if tag != savedTag {
		claimed, previousTag := c.claimTag(deploy, tag)
		if !claimed {
			fmt.Printf("Tag %s for %s/%s already processed, skipping\n", tag, deploy.Namespace, deploy.Name)
			return
		}
		savedTag = previousTag
		name := deploy.Annotations["atomicjolt.com/release-notifier-name"]
		if name == "" {
			name = deploy.Labels["app.kubernetes.io/name"]
		}
		slackmoji := deploy.Annotations["atomicjolt.com/release-notifier-slackmoji"]
		if slackmoji == "" {
			slackmoji = name
		}
		environment := deploy.Annotations["atomicjolt.com/release-notifier-environment"]
		if environment == "" {
			environment = deploy.Namespace
		}
		fmt.Printf("APP UPDATED: %s/%s, Image: %s -> %s\n", deploy.Namespace, name, savedTag, tag)
		message := containerLabel(image)
		fmt.Printf("Tag Message: %s\n", message)
		slackTs := notifySlack(name, deploy.Namespace, environment, tag, savedTag, slackmoji, message)
		notifySheet(name, deploy.Namespace, environment, tag, message)
		//notifyForm(name, deploy.Namespace, environment, tag, message)

		e2eEnv := deploy.Annotations["atomicjolt.com/release-notifier-e2e-environment"]
		e2eLabel := deploy.Annotations["atomicjolt.com/release-notifier-e2e-label"]
		e2eTags := deploy.Annotations["atomicjolt.com/release-notifier-e2e-tags"]
		e2eRef := deploy.Annotations["atomicjolt.com/release-notifier-e2e-ref"]
		e2eLmsDomain := deploy.Annotations["atomicjolt.com/release-notifier-e2e-lms-domain"]

		if e2eRef == "" {
			e2eRef = "main"
		}
		if e2eLmsDomain == "" {
			e2eLmsDomain = "atomicjolt.instructure.com"
		}

		if e2eEnv != "" && e2eLabel != "" && e2eTags != "" {
			fmt.Printf("E2E TESTS >> Environment: %s, Label: %s, Tags: %s, Ref: %s\n", e2eEnv, e2eLabel, e2eTags, e2eRef)
			if err := notifyGithub(e2eEnv, e2eLabel, e2eTags, e2eRef, e2eLmsDomain, slackTs); err != nil {
				fmt.Println("E2E TESTS >>", err)
				replySlack(slackTs, fmt.Sprintf(":warning: Couldn't start the E2E tests: %s", err))
			}
		} else {
			fmt.Println("No E2E test configuration found for deployment.")
		}

		issuesSourceRepo := deploy.Annotations["atomicjolt.com/release-notifier-issues-repo"]
		issuesProjects := deploy.Annotations["atomicjolt.com/release-notifier-issues-projects"]
		issuesStatus := deploy.Annotations["atomicjolt.com/release-notifier-issues-status"]
		issuesEnvironment := deploy.Annotations["atomicjolt.com/release-notifier-issues-environment"]

		if issuesSourceRepo != "" && issuesProjects != "" && issuesStatus != "" {
			if savedTag == "" {
				fmt.Println("ISSUE STATUS >> No previous tag to compare against, skipping")
			} else {
				fmt.Printf("ISSUE STATUS >> Repo: %s, %s...%s, Projects: %s, Status: %s, Environment: %s\n", issuesSourceRepo, savedTag, tag, issuesProjects, issuesStatus, issuesEnvironment)
				if err := markDeployed(issuesSourceRepo, issuesProjects, issuesStatus, issuesEnvironment, tag, savedTag); err != nil {
					fmt.Println("ISSUE STATUS >>", err)
					replySlack(slackTs, fmt.Sprintf(":warning: Couldn't start the %s issue status update: %s", issuesStatus, err))
				}
			}
		}
	}
}

// claimTag atomically records tag as the saved tag on the live Deployment.
// The annotation acts as a lock: the check is made against the latest object
// from the API server and the update is guarded by resourceVersion, so only
// one caller (even across replicas or stale queued events) can claim a given
// tag. Returns whether the tag was claimed and the previously saved tag.
func (c *DeploymentMonitoringController) claimTag(deploy *appsv1.Deployment, tag string) (bool, string) {
	deploymentsClient := c.clientset.AppsV1().Deployments(deploy.Namespace)
	claimed := false
	previousTag := ""
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := deploymentsClient.Get(context.TODO(), deploy.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		previousTag = result.Annotations["atomicjolt.com/release-notifier-saved-tag"]
		if previousTag == tag {
			// Another event already claimed this tag
			claimed = false
			return nil
		}
		if result.Annotations == nil {
			result.Annotations = make(map[string]string)
		}
		result.Annotations["atomicjolt.com/release-notifier-saved-tag"] = tag
		_, updateErr := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		claimed = updateErr == nil
		return updateErr
	})
	if retryErr != nil {
		panic(fmt.Errorf("Update deployment failed: %v", retryErr))
	}
	return claimed, previousTag
}

func (c *DeploymentMonitoringController) deploymentAdd(obj interface{}) {
	deploy := obj.(*appsv1.Deployment)
	fmt.Printf("MONITORING %s/%s\n", deploy.Namespace, deploy.Name)

	if c.deploymentReady(deploy) {
		c.updateDeployment(deploy)
	}
}

func (c *DeploymentMonitoringController) deploymentUpdate(old, new interface{}) {
	oldDeploy := old.(*appsv1.Deployment)
	newDeploy := new.(*appsv1.Deployment)

	newImage := newDeploy.Spec.Template.Spec.Containers[0].Image
	oldImage := oldDeploy.Spec.Template.Spec.Containers[0].Image
	newTag := newImage[strings.LastIndex(newImage, ":")+1:]
	oldTag := oldImage[strings.LastIndex(oldImage, ":")+1:]

	if newTag != oldTag && !c.deploymentReady(newDeploy) {
		fmt.Printf("Waiting for deployment %s/%s to be ready before notifying\n", newDeploy.Namespace, newDeploy.Name)
	}

	if oldDeploy.Annotations["atomicjolt.com/release-notifier-saved-tag"] != newDeploy.Annotations["atomicjolt.com/release-notifier-saved-tag"] {
		// Already notified
		return
	}

	if c.deploymentReady(newDeploy) {
		c.updateDeployment(newDeploy)
	}
}

func (c *DeploymentMonitoringController) deploymentReady(deploy *appsv1.Deployment) bool {
	if deploy.Status.ObservedGeneration != deploy.Generation {
		// Deployment has not yet been observed by the controller
		return false
	}
	if deploy.Spec.Replicas != nil && deploy.Status.UpdatedReplicas == *deploy.Spec.Replicas && deploy.Status.UnavailableReplicas == 0 {
		// Deployment is ready
		return true
	}
	// Waiting for the deployment to be ready
	return false
}

// NewServiceMonitoringController creates a ServiceMonitoringController
func NewDeploymentMonitoringController(informerFactory informers.SharedInformerFactory, clientset kubernetes.Clientset) (*DeploymentMonitoringController, error) {
	deploymentInformer := informerFactory.Apps().V1().Deployments()

	c := &DeploymentMonitoringController{
		informerFactory:    informerFactory,
		deploymentInformer: deploymentInformer,
		clientset:          clientset,
	}
	_, err := deploymentInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			// Called on creation
			AddFunc: c.deploymentAdd,
			// Called on resource update and every resyncPeriod on existing resources.
			UpdateFunc: c.deploymentUpdate,
		},
	)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// Run starts shared informers and waits for the shared informer cache to
// synchronize.
func (c *DeploymentMonitoringController) Run(stopCh chan struct{}) error {
	// Starts all the shared informers that have been created by the factory so
	// far.
	c.informerFactory.Start(stopCh)
	// wait for the initial synchronization of the local cache.
	if !cache.WaitForCacheSync(stopCh, c.deploymentInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to sync")
	}
	return nil
}
