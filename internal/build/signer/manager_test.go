package signer

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/build"
	"github.com/kubernetes-sigs/kernel-module-management/internal/client"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	registrypkg "github.com/kubernetes-sigs/kernel-module-management/internal/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Labels", func() {
	It("should work as expected", func() {
		const (
			moduleName   = "module-name"
			targetKernel = "1.2.3"
			buildStage   = "test"
		)

		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: moduleName},
		}

		labels := labels(mod, targetKernel, buildStage)

		Expect(labels).To(HaveKeyWithValue(constants.ModuleNameLabel, moduleName))
		Expect(labels).To(HaveKeyWithValue(constants.TargetKernelTarget, targetKernel))
		Expect(labels).To(HaveKeyWithValue(constants.BuildStage, buildStage))
	})
})

var _ = Describe("shouldRun", func() {
		DescribeTable("should return true if signing params are defined",
			func(mod *kmmv1beta1.Module, km *kmmv1beta1.KernelMapping, expectsTrue bool) {
				var (
					ctrl     *gomock.Controller
					//clnt     *client.MockClient
					registry *registrypkg.MockRegistry
					maker    *MockSigner
					helper   *build.MockHelper
				)

				//BeforeEach(func() {
					ctrl = gomock.NewController(GinkgoT())
				//clnt = client.NewMockClient(ctrl)
					registry = registrypkg.NewMockRegistry(ctrl)
					maker = NewMockSigner(ctrl)
					helper = build.NewMockHelper(ctrl)
				//})
	

				mgr := NewSigningManager(nil, registry, maker, helper)
				run := mgr.ShouldRun(mod, km)

				if expectsTrue {
					Expect(run).To(Equal(true))
				}else{
					Expect(run).To(Equal(false))
				}
			},
			Entry("kernelmapping",
				&kmmv1beta1.Module{
					Spec: kmmv1beta1.ModuleSpec{
						ModuleLoader: kmmv1beta1.ModuleLoaderSpec{
							Container: kmmv1beta1.ModuleLoaderContainerSpec{},
						},
					},
				},
				&kmmv1beta1.KernelMapping{Sign: &kmmv1beta1.Sign{}},
				true,
			),
			Entry("Container",
				&kmmv1beta1.Module{
					Spec: kmmv1beta1.ModuleSpec{
						ModuleLoader: kmmv1beta1.ModuleLoaderSpec{
							Container: kmmv1beta1.ModuleLoaderContainerSpec{
								Sign: &kmmv1beta1.Sign{},
							},
						},
					},
				},
				&kmmv1beta1.KernelMapping{},
				true,
			),
			Entry("no signig",
				&kmmv1beta1.Module{
					Spec: kmmv1beta1.ModuleSpec{
						ModuleLoader: kmmv1beta1.ModuleLoaderSpec{
							Container: kmmv1beta1.ModuleLoaderContainerSpec{},
						},
					},
				},
				&kmmv1beta1.KernelMapping{},
				false,
			),
		)

})

var _ = Describe("JobManager", func() {
	Describe("Sync", func() {

		var (
			ctrl     *gomock.Controller
			clnt     *client.MockClient
			registry *registrypkg.MockRegistry
			maker    *MockSigner
			helper   *build.MockHelper
		)

		const (
			signedImage = "image-name-signed"
			imageName = "image-name"
			namespace = "some-namespace"
			buildStage   = "test"
		)

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			clnt = client.NewMockClient(ctrl)
			registry = registrypkg.NewMockRegistry(ctrl)
			maker = NewMockSigner(ctrl)
			helper = build.NewMockHelper(ctrl)
		})

		po := &kmmv1beta1.PullOptions{}

		km := kmmv1beta1.KernelMapping{
			Sign:          &kmmv1beta1.Sign{SignedImage: signedImage, Pull: po},
			ContainerImage: imageName,
		}

		It("should return the basename of the job", func() {
			mgr := NewSigningManager(nil, registry, maker, helper)
			gomock.InOrder(
				maker.EXPECT().GetName().Return("sign"),
			)

			Expect(
				mgr.GetName(),
			).To(
				Equal("sign"),
			)
		})

		const (
			moduleName    = "module-name"
			kernelVersion = "1.2.3"
			jobName       = "some-job"
		)

		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: moduleName},
		}

		DescribeTable("should return the correct status depending on the job status",
			func(s batchv1.JobStatus, r build.Result, expectsErr bool) {

				j := batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Labels:    labels(mod, kernelVersion, buildStage),
						Namespace: namespace,
					},
					Status: s,
				}
				ctx := context.Background()
				clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, list *batchv1.JobList, _ ...interface{}) error {
						list.Items = []batchv1.Job{j}
						return nil
					},
				)
			//clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any(), gomock.Any())

				gomock.InOrder(
					helper.EXPECT().GetRelevantSign(mod, km).Return(km.Sign),
					maker.EXPECT().GetName().Return("sign").AnyTimes(),
					maker.EXPECT().GetName().Return("sign").AnyTimes(),
				)
				mgr := NewSigningManager(clnt, registry, maker, helper)

				res, err := mgr.Sync(ctx, mod, km, kernelVersion, true)

				if expectsErr {
					Expect(err).To(HaveOccurred())
					return
				}

				Expect(res).To(Equal(r))
			},
			Entry("active", batchv1.JobStatus{Active: 1}, build.Result{Requeue: true, Status: build.StatusInProgress}, false),
			Entry("succeeded", batchv1.JobStatus{Succeeded: 1}, build.Result{Status: build.StatusCompleted}, false),
			Entry("failed", batchv1.JobStatus{Failed: 1}, build.Result{}, true),
		)

		It("should return an error if there was an error creating the job", func() {
			ctx := context.Background()

			gomock.InOrder(
				helper.EXPECT().GetRelevantSign(mod, km).Return(km.Sign),
				maker.EXPECT().GetName().Return("sign").AnyTimes(),
				maker.EXPECT().GetName().Return("sign").AnyTimes(),
				maker.EXPECT().MakeJob(mod, km.Sign, kernelVersion, true).Return(nil, errors.New("random error")),
			)
			clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any(), gomock.Any())
			//clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any(), gomock.Any())

			mgr := NewSigningManager(clnt, registry, maker, helper)

			Expect(
				mgr.Sync(ctx, mod, km, kernelVersion, true),
			).Error().To(
				HaveOccurred(),
			)
		})

		It("should create the job if there was no error making it", func() {
			ctx := context.Background()

			j := batchv1.Job{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "batch/v1",
					Kind:       "Job",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: namespace,
				},
			}

			gomock.InOrder(
				helper.EXPECT().GetRelevantSign(mod, km).Return(km.Sign),
				maker.EXPECT().GetName().Return("sign").AnyTimes(),
				maker.EXPECT().GetName().Return("sign").AnyTimes(),
				maker.EXPECT().MakeJob(mod, km.Sign, kernelVersion, true).Return(&j, nil),
			)

			gomock.InOrder(
				clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any(), gomock.Any()),
				clnt.EXPECT().Create(ctx, &j),
				//clnt.EXPECT().List(ctx, gomock.Any(), gomock.Any(), gomock.Any()),
			)

			mgr := NewSigningManager(clnt, registry, maker, helper)

			Expect(
				mgr.Sync(ctx, mod, km, kernelVersion, true),
			).To(
				Equal(build.Result{Requeue: true, Status: build.StatusCreated}),
			)
		})

	})
})
