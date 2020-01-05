package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	credentialmanagerv1alpha1 "github.com/mgoltzsche/credential-manager/pkg/apis/credentialmanager/v1alpha1"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type Authenticated struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type ErrorLogger func(error)

type Authenticator struct {
	client client.Client
	cache  map[string]*credentialmanagerv1alpha1.ImagePullSecret
	lock   sync.Locker
	log    ErrorLogger
}

func NewAuthenticator(log ErrorLogger) (a *Authenticator, err error) {
	cfg := &rest.Config{}
	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return
	}
	scheme, err := credentialmanagerv1alpha1.SchemeBuilder.Build()
	if err != nil {
		return
	}
	reader, err := client.New(cfg, client.Options{Scheme: scheme, Mapper: mapper})
	if err != nil {
		return
	}
	return &Authenticator{reader, map[string]*credentialmanagerv1alpha1.ImagePullSecret{}, &sync.Mutex{}, log}, nil
}

func (a *Authenticator) Authenticate(user, passwd string) *Authenticated {
	cr := a.findCR(user)
	if isValid(cr) && matchPassword(cr.Status.Passwords, passwd) {
		return &Authenticated{
			Namespace: cr.Namespace,
			Name:      cr.Name,
		}
	}
	return nil
}

func matchPassword(hashed []string, passwd string) bool {
	for _, h := range hashed {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(passwd)) == nil {
			return true
		}
	}
	return false
}

func (a *Authenticator) findCR(user string) (cr *credentialmanagerv1alpha1.ImagePullSecret) {
	userParts := strings.SplitN(user, "/", 3)
	if len(userParts) != 3 {
		return // unsupported user name format
	}
	if cr = a.cache[user]; cr != nil {
		return // cached
	}
	namespace := userParts[0]
	name := userParts[1]
	fetchedCR := &credentialmanagerv1alpha1.ImagePullSecret{}
	err := a.client.Get(context.TODO(), types.NamespacedName{namespace, name}, fetchedCR)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			a.log(err)
		}
	} else if isValid(cr) {
		cr = fetchedCR
		a.addToCache(user, cr)
	}
	return
}

func (a *Authenticator) addToCache(user string, cr *credentialmanagerv1alpha1.ImagePullSecret) {
	a.lock.Lock()
	defer a.lock.Unlock()
	cache := map[string]*credentialmanagerv1alpha1.ImagePullSecret{}
	for usr, cr := range a.cache {
		if isValid(cr) {
			// drop old entries
			cache[usr] = cr
		}
	}
	cache[user] = cr
	a.cache = cache
}

func isValid(cr *credentialmanagerv1alpha1.ImagePullSecret) bool {
	return time.Now().Before(cr.Status.GenerationDate.Add(time.Minute * 30))
}
