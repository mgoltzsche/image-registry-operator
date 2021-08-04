package auth

import (
	"context"
	"sync"
	"time"

	registryapi "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	Origin = "cr"
)

var originCR = []string{Origin}

type cachedAccount struct {
	HashedPassword
	Labels   map[string][]string
	LastSeen time.Time
}

func (a *cachedAccount) CacheExpired() bool {
	return time.Now().After(a.LastSeen.Add(10 * time.Minute))
}

type HashedPassword string

func (h HashedPassword) MatchPassword(pw string) bool {
	if bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) == nil {
		return true
	}
	return false
}

type ErrorLogger func(error)

type Authenticator struct {
	client    client.Client
	cache     map[string]*cachedAccount
	lock      sync.Locker
	log       ErrorLogger
	namespace string
}

func NewAuthenticator(cfg *rest.Config, namespace string, log ErrorLogger) (a *Authenticator, err error) {
	scheme, err := registryapi.SchemeBuilder.Build()
	if err != nil {
		return
	}
	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return
	}
	reader, err := client.New(cfg, client.Options{Scheme: scheme, Mapper: mapper})
	if err != nil {
		return
	}
	return &Authenticator{reader, map[string]*cachedAccount{}, &sync.Mutex{}, log, namespace}, nil
}

func (a *Authenticator) Authenticate(user, passwd string) (labels map[string][]string, err error) {
	if user != "" && passwd != "" {
		var account *cachedAccount
		account, err = a.findAccount(user)
		if err == nil && account != nil && account.MatchPassword(passwd) {
			labels = account.Labels
		}
	}
	return
}

func (a *Authenticator) findAccount(username string) (account *cachedAccount, _ error) {
	if account = a.cache[username]; account != nil && !account.CacheExpired() {
		return // cached
	}

	key := types.NamespacedName{Name: username, Namespace: a.namespace}
	acc := &registryapi.ImageRegistryAccount{}
	err := a.client.Get(context.TODO(), key, acc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	labels := map[string][]string{}
	for k, v := range acc.Spec.Labels {
		labels[k] = v
	}
	labels["origin"] = originCR
	labels["account"] = []string{acc.Name}
	account = &cachedAccount{
		HashedPassword: HashedPassword(acc.Spec.Password),
		Labels:         labels,
		LastSeen:       time.Now(),
	}
	a.addToCache(username, account)
	return
}

func (a *Authenticator) addToCache(username string, account *cachedAccount) {
	a.lock.Lock()
	defer a.lock.Unlock()
	cache := map[string]*cachedAccount{}
	// drop old entries
	for usr, cached := range a.cache {
		if !cached.CacheExpired() {
			cache[usr] = cached
		}
	}
	cache[username] = account
	a.cache = cache
}
