/*
Copyright 2022 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resolutionrequest

import (
	"context"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	resolutionrequestinformer "github.com/tektoncd/pipeline/pkg/client/resolution/injection/informers/resolution/v1beta1/resolutionrequest"
	resolutionrequestreconciler "github.com/tektoncd/pipeline/pkg/client/resolution/injection/reconciler/resolution/v1beta1/resolutionrequest"
	"k8s.io/utils/clock"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController returns a func that returns a knative controller for processing
// ResolutionRequest objects.
func NewController(clock clock.PassiveClock) func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)

		configStore := config.NewStore(logger.Named("config-store"))
		configStore.WatchConfigs(cmw)

		r := &Reconciler{
			clock: clock,
		}
		impl := resolutionrequestreconciler.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options {
			return controller.Options{
				ConfigStore: configStore,
			}
		})

		reqinformer := resolutionrequestinformer.Get(ctx)
		if _, err := reqinformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue)); err != nil {
			logging.FromContext(ctx).Panicf("Couldn't register ResolutionRequest informer event handler: %w", err)
		}

		return impl
	}
}
