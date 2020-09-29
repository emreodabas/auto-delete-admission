package autodelete

import (
	"fmt"
	admissioncontrol "github.com/elithrar/admission-control"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/api/batch/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"strings"
)

const (
	AutoDeleteAnnotation = "auto-delete-admission"
)

func AutoDelete(ignoredNamespaces []string) admissioncontrol.AdmitFunc {
	return func(admissionReview *admission.AdmissionReview) (*admission.AdmissionResponse, error) {
		resp := newDefaultDenyResponse()
		deserializer := serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
		unstructureds := unstructured.Unstructured{}
		if _, _, err := deserializer.Decode(admissionReview.Request.Object.Raw, nil, &unstructureds); err != nil {
			return newDefaultDenyResponse(), err
		}
		if ok := ensureHasAnnotationKey(AutoDeleteAnnotation, unstructureds.GetAnnotations()); !ok {
			return newDefaultSuccessResponse(admissionReview.Request.UID), nil
		}
		value := unstructureds.GetAnnotations()[AutoDeleteAnnotation]
		fmt.Println(value)
		createCronJob(unstructureds.GetGenerateName(), value)

		resp.Allowed = true
		return resp, nil
	}
}

func createCronJob(name string, value string) *v1beta1.CronJob {
	cronJob := new(v1beta1.CronJob)
	cronJob.ObjectMeta = *createObjectMeta(name)
	return cronJob
}

func newDefaultDenyResponse() *admission.AdmissionResponse {
	return &admission.AdmissionResponse{
		Allowed: false,
		Result:  &metav1.Status{},
	}
}

func newDefaultSuccessResponse(uid types.UID) *admission.AdmissionResponse {
	return &admission.AdmissionResponse{
		Allowed: true,
		UID:     uid,
	}

}
func ensureHasAnnotations(required map[string]string, annotations map[string]string) (map[string]string, bool) {
	missing := make(map[string]string)
	for requiredKey, requiredVal := range required {
		if existingVal, ok := annotations[requiredKey]; !ok {
			// Missing a required annotation; add it to the list
			missing[requiredKey] = requiredVal
		} else {
			// The key exists; does the value match?
			if existingVal != requiredVal {
				missing[requiredKey] = requiredVal
			}
		}
	}

	// If we have any missing annotations, report them to the caller so the user
	// can take action.
	if len(missing) > 0 {
		return missing, false
	}

	return nil, true
}

func ensureHasAnnotationKey(required string, annotations map[string]string) bool {
	if _, ok := annotations[required]; !ok {
		return false
	}
	return true
}

func createObjectMeta(name string) *metav1.ObjectMeta {
	objectMeta := new(metav1.ObjectMeta)
	objectMeta.Name = strings.ToLower(name)
	objectMeta.Labels = map[string]string{
		"app":  strings.ToLower(name),
		"type": "user-app",
	}
	return objectMeta
}
