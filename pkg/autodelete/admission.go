package autodelete

import (
	"encoding/json"
	admissioncontrol "github.com/elithrar/admission-control"
	admission "k8s.io/api/admission/v1beta1"
	v12 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"os"
	"strings"
)

const (
	AutoDeleteAnnotation = "auto-delete-admission"
	ContainerImage       = "busybox:latest"
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
		job := createCronJob(unstructureds.GetGenerateName(), parseScheduleValue(value), unstructureds.GetName()+unstructureds.GetKind())

		patchOperationsBytes, err := json.Marshal(job)

		if err != nil {
			resp.Result.Code = http.StatusInternalServerError
			resp.Allowed = false
			return resp, err
		}
		resp.Allowed = true
		resp.Patch = patchOperationsBytes
		return resp, nil
	}
}

func parseScheduleValue(value string) string {
	//TODO
	return value
}

func createCronJob(name string, schedule string, resource string) *v1beta1.CronJob {
	cronJob := new(v1beta1.CronJob)
	cronJob.ObjectMeta = *createObjectMeta(name)
	cronJob.Spec = *createSpec(resource, schedule)
	return cronJob
}

func createSpec(resource string, schedule string) *v1beta1.CronJobSpec {
	spec := new(v1beta1.CronJobSpec)
	spec.Schedule = schedule
	spec.JobTemplate = *createJobTemplate(resource)

	return spec
}

func createJobTemplate(resource string) *v1beta1.JobTemplateSpec {
	jobTemplateSpec := new(v1beta1.JobTemplateSpec)
	jobTemplateSpec.Spec = *createJobSpec(resource)
	return jobTemplateSpec
}

func createJobSpec(resource string) *v12.JobSpec {
	jobSpec := new(v12.JobSpec)
	jobSpec.Template = *createPodTemplateSpec(resource)
	return jobSpec
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
	objectMeta.Namespace = os.Getenv("NAMESPACE")
	objectMeta.Labels = map[string]string{
		"admission-controller-name": strings.ToLower(name),
		"type":                      "auto-delete-cronjob",
	}
	return objectMeta
}

func createPodTemplateSpec(resource string) *v1.PodTemplateSpec {
	podTemplateSpec := new(v1.PodTemplateSpec)
	podTemplateSpecObjectMeta := new(metav1.ObjectMeta)
	podTemplateSpec.ObjectMeta = *podTemplateSpecObjectMeta
	podSpec := createPodSpec(resource)
	podTemplateSpec.Spec = *podSpec
	return podTemplateSpec
}

func createPodSpec(resource string) *v1.PodSpec {
	container := createContainer(resource)
	podSpec := new(v1.PodSpec)
	podSpec.Containers = []v1.Container{*container}
	return podSpec
}

func createContainer(resourceName string) *v1.Container {
	container := new(v1.Container)
	container.Name = "auto-delete"
	container.Image = ContainerImage
	container.Command = []string{"curl", "$service", resourceName}
	container.ImagePullPolicy = v1.PullIfNotPresent
	return container
}
