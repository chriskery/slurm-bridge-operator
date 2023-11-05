package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func GetMetaTimePointer(time time.Time) *metav1.Time {
	newTime := metav1.NewTime(time)
	return &newTime
}
