package imagebuildenv

import (
	"context"
	"fmt"
	"time"

	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/backrefs"
	"github.com/mgoltzsche/image-registry-operator/pkg/merge"
	"github.com/mgoltzsche/image-registry-operator/pkg/passwordgen"
	"github.com/mgoltzsche/image-registry-operator/pkg/registriesconf"
	"github.com/operator-framework/operator-sdk/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_imagebuildenv")

const (
	redisPort           = 6379
	redisPortName       = "redis"
	secretKeyConfigJson = "config.json"
	finalizer           = "registry.mgoltzsche.github.com/inputrefs"
)

// Add creates a new ImageBuildEnv Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &ReconcileImageBuildEnv{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		secretRefs: backrefs.NewBackReferencesHandler(mgr.GetClient(), backrefs.OwnerReferences()),
	}

	// Create a new controller
	c, err := controller.New("imagebuildenv-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ImageBuildEnv
	err = c.Watch(&source.Kind{Type: &registryv1alpha1.ImageBuildEnv{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resources
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageBuildEnv{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &registryv1alpha1.ImageBuildEnv{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &registryv1alpha1.ImageBuildEnv{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileImageBuildEnv implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileImageBuildEnv{}

// ReconcileImageBuildEnv reconciles a ImageBuildEnv object
type ReconcileImageBuildEnv struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	secretRefs *backrefs.BackReferencesHandler
}

// Reconcile reads that state of the cluster for a ImageBuildEnv object and makes changes based on the state read
// and what is in the ImageBuildEnv.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileImageBuildEnv) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ImageBuildEnv")

	// Fetch the ImageBuildEnv instance
	instance := &registryv1alpha1.ImageBuildEnv{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// When marked as deleted finalize object: remove back references
	isFinalizerPresent := merge.HasFinalizer(instance, finalizer)
	refOwner := &referenceOwner{instance}
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if isFinalizerPresent {
			err = r.secretRefs.UpdateReferences(context.TODO(), reqLogger, refOwner, nil)
			if err != nil {
				reqLogger.Error(err, "finalizer %s failed to remove input ownerReferences", finalizer)
				return reconcile.Result{}, err
			}
			controllerutil.RemoveFinalizer(instance, finalizer)
			err = r.client.Update(context.TODO(), instance)
		}
		return reconcile.Result{}, err
	}

	// Add finalizer
	if !isFinalizerPresent {
		controllerutil.AddFinalizer(instance, finalizer)
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonFailedUpdate, err.Error())
			return reconcile.Result{}, err
		}
		// Stop here since update triggered another reconcile request anyway
		return reconcile.Result{}, nil
	}

	// Load referenced docker config secrets
	secrets, err := r.loadInputSecretsForCR(instance)
	if err != nil {
		// secret not found - reconcile after one minute
		err = r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonMissingSecret, err.Error())
		return reconcile.Result{RequeueAfter: 30 * time.Second}, err
	}
	err = r.secretRefs.UpdateReferences(context.TODO(), reqLogger, refOwner, secretsToObjects(secrets))
	if err != nil {
		err = r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonFailedUpdate, err.Error())
		return reconcile.Result{}, err
	}

	// Merge the secrets
	data, err := r.mergeSecretData(secrets)
	if err != nil {
		// config invalid
		err = r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonInvalidSecret, err.Error())
		return reconcile.Result{}, err
	}

	// Configure redis and upsert output Secret
	ready, err := r.configureRedis(instance, data)
	if err == nil {
		if !ready {
			err = r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonPending, "waiting for redis to become ready")
			return reconcile.Result{}, err
		}

		err = r.upsertMergedSecretForCR(instance, data)
	}
	if err != nil {
		err = r.updateStatus(instance, corev1.ConditionFalse, registryv1alpha1.ReasonFailedUpdate, err.Error())
		return reconcile.Result{}, err
	}

	err = r.updateStatus(instance, corev1.ConditionTrue, "", "")
	return reconcile.Result{}, err
}

func secretsToObjects(secrets []*corev1.Secret) []backrefs.Object {
	r := make([]backrefs.Object, len(secrets))
	for i, s := range secrets {
		r[i] = s
	}
	return r
}

func (r *ReconcileImageBuildEnv) updateStatus(cr *registryv1alpha1.ImageBuildEnv, ready corev1.ConditionStatus, reason status.ConditionReason, msg string) error {
	c := status.Condition{
		Type:    registryv1alpha1.ConditionReady,
		Status:  ready,
		Reason:  reason,
		Message: msg,
	}
	if cr.Status.Conditions.SetCondition(c) {
		return r.client.Status().Update(context.TODO(), cr)
	}
	return nil
}

func (r *ReconcileImageBuildEnv) upsertMergedSecretForCR(cr *registryv1alpha1.ImageBuildEnv, data map[string][]byte) (err error) {
	mergedSecret := &corev1.Secret{}
	mergedSecret.Name = "imagebuildenv-" + cr.Name + "-conf"
	mergedSecret.Namespace = cr.Namespace
	mergedSecret.Type = corev1.SecretTypeOpaque
	if err = controllerutil.SetControllerReference(cr, mergedSecret, r.scheme); err != nil {
		return
	}
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, mergedSecret, func() error {
		mergedSecret.Data = data
		return nil
	})
	return
}

func (r *ReconcileImageBuildEnv) loadInputSecretsForCR(cr *registryv1alpha1.ImageBuildEnv) (secrets []*corev1.Secret, err error) {
	secrets = make([]*corev1.Secret, len(cr.Spec.Secrets))
	for i, s := range cr.Spec.Secrets {
		key := types.NamespacedName{Name: s.SecretName, Namespace: cr.Namespace}
		secret := &corev1.Secret{}
		err = r.client.Get(context.TODO(), key, secret)
		if err != nil {
			return
		}
		secrets[i] = secret
	}
	return
}

func (r *ReconcileImageBuildEnv) mergeSecretData(secrets []*corev1.Secret) (data map[string][]byte, err error) {
	var (
		makisuConf registriesconf.MakisuRegistries = map[string]registriesconf.MakisuRepos{}
		dockerConf                                 = &registriesconf.DockerConfig{Auths: map[string]registriesconf.DockerConfigUrlAuth{}}
		inputConf  *registriesconf.DockerConfig
	)
	for _, secret := range secrets {
		inputConf, err = dockerConfigFromSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("secret %s: %w", secret.Name, err)
		}
		// Merge config
		if inputConf.Auths != nil {
			for k, v := range inputConf.Auths {
				dockerConf.Auths[k] = v
				auth, e := registriesconf.ToMakisuBasicAuth(v.Auth)
				if e != nil {
					return nil, fmt.Errorf("secret %s basic auth: %w", secret.Name, e)
				}
				makisuConf.AddRegistry(k, ".*", auth)
			}
		}
	}

	// Prepare secret data
	data = map[string][]byte{}
	if len(secrets) > 0 {
		for k, v := range secrets[0].Data {
			data[k] = v
		}
	}
	data[corev1.DockerConfigJsonKey] = dockerConf.JSON()
	data[registryv1alpha1.SecretKeyMakisuYAML] = makisuConf.YAML()
	return data, nil
}

func dockerConfigFromSecret(secret *corev1.Secret) (*registriesconf.DockerConfig, error) {
	var configJson []byte
	if secret.Data == nil {
		return nil, fmt.Errorf("secret %s does not specify data", secret.Name)
	}
	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		configJson = secret.Data[corev1.DockerConfigJsonKey]
		if len(configJson) == 0 {
			return nil, fmt.Errorf("secret %s does not specify key %q", secret.Name, corev1.DockerConfigJsonKey)
		}
	case corev1.SecretTypeOpaque:
		configJson = secret.Data[secretKeyConfigJson]
		if len(configJson) == 0 {
			return nil, fmt.Errorf("secret %s does not specify key %q", secret.Name, secretKeyConfigJson)
		}
	default:
		return nil, fmt.Errorf("secret %s has unsupported type %s, expected %s or %s", secret.Name, secret.Type, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeOpaque)
	}
	return registriesconf.ParseDockerConfig(configJson)
}

func (r *ReconcileImageBuildEnv) configureRedis(cr *registryv1alpha1.ImageBuildEnv, data map[string][]byte) (ready bool, err error) {
	if cr.Spec.Redis {
		var password []byte
		password, err = r.upsertRedisSecretForCR(cr)
		if err != nil {
			return
		}
		data[registryv1alpha1.SecretKeyRedis] = []byte(fmt.Sprintf("%s:%d", redisNameForCR(cr), redisPort))
		data[registryv1alpha1.SecretKeyRedisPassword] = password
		ready, err = r.createRedisPodForCR(cr)
		if err != nil {
			return
		}
		err = r.upsertRedisServiceForCR(cr)
	} else {
		ready = true
		// TODO: delete redis pod+svc
	}
	return
}

func (r *ReconcileImageBuildEnv) createRedisPodForCR(cr *registryv1alpha1.ImageBuildEnv) (ready bool, err error) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Name: redisNameForCR(cr), Namespace: cr.Namespace}
	err = r.client.Get(context.TODO(), podKey, pod)
	if err == nil || !errors.IsNotFound(err) {
		return isPodReady(pod), err
	}
	pod.Name = podKey.Name
	pod.Namespace = cr.Namespace
	pod.Labels = redisLabelsForCR(cr)
	merge.AddVolume(&pod.Spec, corev1.Volume{
		Name: "redis-conf",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: pod.Name + "-conf"},
		},
	})
	merge.AddContainer(&pod.Spec, corev1.Container{
		Name:            "redis",
		Image:           "redis:6-alpine",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"redis-server", "/conf/redis.conf"},
		Ports: []corev1.ContainerPort{
			{Name: redisPortName, ContainerPort: redisPort, Protocol: corev1.ProtocolTCP},
		},
		Env: []corev1.EnvVar{
			{Name: "MASTER", Value: "true"},
		},
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.IntOrString{Type: intstr.Int, IntVal: redisPort},
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       3,
		},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.IntOrString{Type: intstr.Int, IntVal: redisPort},
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       30,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "redis-conf",
				MountPath: "/conf",
			},
		},
	})
	err = controllerutil.SetControllerReference(cr, pod, r.scheme)
	if err != nil {
		return
	}
	return false, r.client.Create(context.TODO(), pod)
}

func (r *ReconcileImageBuildEnv) upsertRedisServiceForCR(cr *registryv1alpha1.ImageBuildEnv) (err error) {
	svc := &corev1.Service{}
	svc.Name = redisNameForCR(cr)
	svc.Namespace = cr.Namespace
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, svc, func() error {
		svc.Spec.Selector = redisLabelsForCR(cr)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		merge.AddServicePort(svc, redisPortName, redisPort, redisPort, corev1.ProtocolTCP)
		return controllerutil.SetControllerReference(cr, svc, r.scheme)
	})
	return
}

func (r *ReconcileImageBuildEnv) upsertRedisSecretForCR(cr *registryv1alpha1.ImageBuildEnv) (password []byte, err error) {
	secret := &corev1.Secret{}
	secret.Name = redisNameForCR(cr) + "-conf"
	secret.Namespace = cr.Namespace
	secret.Type = corev1.SecretTypeOpaque
	err = controllerutil.SetControllerReference(cr, secret, r.scheme)
	if err != nil {
		return
	}
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, secret, func() error {
		if secret.Data != nil && len(secret.Data["redis_password"]) > 0 {
			password = secret.Data[registryv1alpha1.SecretKeyRedisPassword]
		} else {
			password = passwordgen.GeneratePassword()
		}
		secret.Data = map[string][]byte{
			registryv1alpha1.SecretKeyRedisPassword: password,
			"redis.conf":                            []byte("requirepass " + string(password)),
		}
		return nil
	})
	return
}

func isPodReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func redisNameForCR(cr *registryv1alpha1.ImageBuildEnv) string {
	return "imagebuildenv-" + cr.Name + "-redis"
}

func redisLabelsForCR(cr *registryv1alpha1.ImageBuildEnv) map[string]string {
	return map[string]string{"app": redisNameForCR(cr)}
}
