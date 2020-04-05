package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const AuthType = "cr"

type Authenticated struct {
	Type      string                      `json:"type"`
	Namespace string                      `json:"namespace"`
	Name      string                      `json:"name"`
	Intent    registryapi.ImageSecretType `json:"intent"`
}

type HashedPasswords []string

func (l HashedPasswords) MatchPassword(pw string) bool {
	for _, h := range l {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) == nil {
			return true
		}
	}
	return false
}

type authRecord struct {
	HashedPasswords
	RotationDate  time.Time
	Authenticated Authenticated
}

type ErrorLogger func(error)

type Authenticator struct {
	client client.Client
	cache  map[string]*authRecord
	lock   sync.Locker
	log    ErrorLogger
}

func NewAuthenticator(cfg *rest.Config, log ErrorLogger) (a *Authenticator, err error) {
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
	return &Authenticator{reader, map[string]*authRecord{}, &sync.Mutex{}, log}, nil
}

func (a *Authenticator) Authenticate(user, passwd string) *Authenticated {
	account := a.findAccount(user)
	if account != nil && account.MatchPassword(passwd) {
		return &account.Authenticated
	}
	return nil
}

func (a *Authenticator) findAccount(username string) (account *authRecord) {
	userParts := strings.SplitN(username, "/", 4)
	if len(userParts) != 4 {
		return // unsupported user name format
	}
	if account = a.cache[username]; account != nil {
		return // cached
	}
	namespace := userParts[0]
	name := userParts[1]
	sType := registryapi.ImageSecretType(userParts[2])
	var fetchedCR registryapi.ImageSecret
	switch sType {
	case registryapi.TypePull:
		fetchedCR = &registryapi.ImagePullSecret{}
	case registryapi.TypePush:
		fetchedCR = &registryapi.ImagePushSecret{}
	default:
		return // unsupported secret type
	}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	err := a.client.Get(context.TODO(), key, fetchedCR)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			a.log(err)
		}
	} else if a.notExpired(fetchedCR.GetStatus().RotationDate.Time) {
		account = &authRecord{
			HashedPasswords: HashedPasswords(fetchedCR.GetStatus().Passwords),
			RotationDate:    fetchedCR.GetStatus().RotationDate.Time,
			Authenticated: Authenticated{
				Type:      AuthType,
				Namespace: fetchedCR.GetNamespace(),
				Name:      fetchedCR.GetName(),
				Intent:    sType,
			},
		}
		a.addToCache(username, account)
	}
	return
}

func (a *Authenticator) addToCache(username string, account *authRecord) {
	a.lock.Lock()
	defer a.lock.Unlock()
	cache := map[string]*authRecord{}
	// drop old entries
	for usr, cached := range a.cache {
		if a.notExpired(cached.RotationDate) {
			cache[usr] = cached
		}
	}
	cache[username] = account
	a.cache = cache
}

func (a *Authenticator) notExpired(rotationDate time.Time) bool {
	return time.Now().Before(rotationDate.Add(time.Minute * 30))
}
