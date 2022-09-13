package signer

import (
	"fmt"
	"strings"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/build"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

////go:generate mockgen -source=signer.go -package=signer -destination=mock_signer.go

type Signer interface {
	MakeJob(mod kmmv1beta1.Module, signConfig *kmmv1beta1.Sign, targetKernel string, pushImage bool) (*batchv1.Job, error)
	GetName() string 
}

type signer struct {
	name   string
	helper build.Helper
	scheme *runtime.Scheme
}

func NewSigner(helper build.Helper, scheme *runtime.Scheme) Signer {
	return &signer{name: "sign", helper: helper, scheme: scheme}
}

func (m *signer) GetName() string {
	return m.name
}

func (m *signer) MakeJob(mod kmmv1beta1.Module, signConfig *kmmv1beta1.Sign, targetKernel string, pushImage bool) (*batchv1.Job, error) {
	var args []string

	//signConfig := km.Sign

	//targetImage, err := m.GetOutputImage(mod, km)
	//if err != nil {
	//	return nil, err
	//}

	if pushImage {
		args = []string{"-signedimage", signConfig.SignedImage}
	} else {
		args = append(args, "-no-push")
	}

	args = append(args, "-unsignedimage", signConfig.UnsignedImage)
	args = append(args, "-pullsecret", "/docker_config/config.json")
	args = append(args, "-key", "/signingkey/key.priv")
	args = append(args, "-cert", "/signingcert/public.der")
	if len(signConfig.FilesToSign) > 0 {
		args = append(args, "-filestosign", strings.Join(signConfig.FilesToSign, ":"))
	}
	volumes := []v1.Volume{}
	volumeMounts := []v1.VolumeMount{}


	volumes = append(volumes, m.makeImageSigningSecretVolume(signConfig.KeySecret, "key", "key.priv"))
	volumes = append(volumes, m.makeImageSigningSecretVolume(signConfig.CertSecret, "cert", "public.der"))

	volumeMounts = append(volumeMounts, m.makeImageSigningSecretVolumeMount(signConfig.CertSecret, "/signingcert"))
	volumeMounts = append(volumeMounts, m.makeImageSigningSecretVolumeMount(signConfig.KeySecret, "/signingkey"))

	if mod.Spec.ImageRepoSecret != nil {
		volumes = append(volumes, m.makeImagePullSecretVolume(mod.Spec.ImageRepoSecret))
		volumeMounts = append(volumeMounts, m.makeImagePullSecretVolumeMount(mod.Spec.ImageRepoSecret))
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mod.Name + "-" + m.GetName() +"-",
			Namespace:    mod.Namespace,
			Labels:       labels(mod, targetKernel, m.GetName()),
		},
		Spec: batchv1.JobSpec{
			Completions: pointer.Int32(1),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:         "signimage",
							Image:        "quay.io/chrisp262/kmod-signer:latest",
							Args:         args,
							VolumeMounts: volumeMounts,
						},
					},
					RestartPolicy: v1.RestartPolicyOnFailure,
					Volumes:       volumes,
					NodeSelector:  mod.Spec.Selector,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(&mod, job, m.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return job, nil
}

func (m *signer) makeImageSigningSecretVolume(secretRef *v1.LocalObjectReference, key string, path string) v1.Volume {

	return v1.Volume{
		Name: m.volumeNameFromSecretRef(*secretRef),
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secretRef.Name,
				Items: []v1.KeyToPath{
					{
						Key:  key,
						Path: path,
					},
				},
			},
		},
	}
}

func (m *signer) makeImageSigningSecretVolumeMount(secretRef *v1.LocalObjectReference, mountpoint string) v1.VolumeMount {

	return v1.VolumeMount{
		Name:      m.volumeNameFromSecretRef(*secretRef),
		ReadOnly:  true,
		MountPath: mountpoint,
	}
}

func (m *signer) makeImagePullSecretVolume(secretRef *v1.LocalObjectReference) v1.Volume {

	if secretRef == nil {
		return v1.Volume{}
	}

	return v1.Volume{
		Name: m.volumeNameFromSecretRef(*secretRef),
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secretRef.Name,
				Items: []v1.KeyToPath{
					{
						Key:  v1.DockerConfigJsonKey,
						Path: "config.json",
					},
				},
			},
		},
	}
}

func (m *signer) makeImagePullSecretVolumeMount(secretRef *v1.LocalObjectReference) v1.VolumeMount {

	if secretRef == nil {
		return v1.VolumeMount{}
	}

	return v1.VolumeMount{
		Name:      m.volumeNameFromSecretRef(*secretRef),
		ReadOnly:  true,
		MountPath: "/docker_config",
	}
}

func (m *signer) volumeNameFromSecretRef(ref v1.LocalObjectReference) string {
	return "secret-" + ref.Name
}
