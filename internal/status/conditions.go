package status

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclock "k8s.io/apimachinery/pkg/util/clock"
)

var clock kubeclock.Clock = &kubeclock.RealClock{}

func NewConditions(generation int64, conditions *[]metav1.Condition, logger logr.Logger) *Conditions {
	return &Conditions{
		generation: generation,
		conditions: conditions,
		log:        logger,
	}
}

type Condition interface {
	True(reason, msg string)
	False(reason, msg string)
}

type Conditions struct {
	generation    int64
	conditions    *[]metav1.Condition
	newConditions []*conditionImpl
	log           logr.Logger
}

func (c *Conditions) Condition(condType string) Condition {
	newCond := &conditionImpl{
		conditionType: condType,
		generation:    c.generation,
	}
	c.newConditions = append(c.newConditions, newCond)
	return newCond
}

func (c *Conditions) Apply() (changed bool) {
	for _, cond := range c.newConditions {
		if cond.condition != nil {
			if condChanged := SetCondition(c.conditions, *cond.condition); condChanged {
				c.log.Info("Changing condition", "conditionType", cond.condition.Type, "conditionStatus", cond.condition.Status, "conditionReason", cond.condition.Reason, "conditionMessage", cond.condition.Message)
				changed = true
			}
		}
	}
	return
}

type conditionImpl struct {
	generation    int64
	conditionType string
	condition     *metav1.Condition
}

func (c *conditionImpl) True(reason, msg string) {
	c.set(metav1.ConditionTrue, reason, msg)
}

func (c *conditionImpl) False(reason, msg string) {
	c.set(metav1.ConditionFalse, reason, msg)
}

func (c *conditionImpl) set(status metav1.ConditionStatus, reason, msg string) {
	c.condition = &metav1.Condition{
		Type:               c.conditionType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: c.generation,
	}
}

// SetCondition adds or updates a condition
func SetCondition(conditions *[]metav1.Condition, newCond metav1.Condition) bool {
	newCond.LastTransitionTime = metav1.Time{Time: clock.Now()}

	for i, condition := range *conditions {
		if condition.Type == newCond.Type {
			if condition.Status == newCond.Status {
				newCond.LastTransitionTime = condition.LastTransitionTime
			}
			changed := condition.Status != newCond.Status ||
				condition.ObservedGeneration != newCond.ObservedGeneration ||
				condition.Reason != newCond.Reason ||
				condition.Message != newCond.Message
			(*conditions)[i] = newCond
			return changed
		}
	}
	*conditions = append(*conditions, newCond)
	return true
}

// GetCondition returns the condition of the given type from the list of conditions or nil if it doesn't exist
func GetCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == condType {
			return &condition
		}
	}
	return nil
}
