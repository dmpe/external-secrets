/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

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
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	esv1alpha1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1alpha1"
	esv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	"github.com/external-secrets/external-secrets/pkg/controllers/crds"
)

const (
	errCreateWebhook = "unable to create webhook"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = esv1beta1.AddToScheme(scheme)
	_ = esv1alpha1.AddToScheme(scheme)
}

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Webhook implementation for ExternalSecrets and SecretStores.",
	Long: `Webhook implementation for ExternalSecrets and SecretStores.
	For more information visit https://external-secrets.io`,
	Run: func(cmd *cobra.Command, args []string) {
		var lvl zapcore.Level
		err := lvl.UnmarshalText([]byte(loglevel))
		if err != nil {
			setupLog.Error(err, "error unmarshalling loglevel")
			os.Exit(1)
		}
		c := crds.CertInfo{
			CertDir:  certDir,
			CertName: "tls.crt",
			KeyName:  "tls.key",
			CAName:   "ca.crt",
		}

		logger := zap.New(zap.Level(lvl))
		ctrl.SetLogger(logger)

		setupLog.Info("validating certs")
		err = crds.CheckCerts(c, dnsName, time.Now().Add(time.Hour))
		if err != nil {
			setupLog.Error(err, "error checking certs")
			os.Exit(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		go func(c crds.CertInfo, dnsName string, every time.Duration) {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			ticker := time.NewTicker(every)
			for {
				select {
				case <-sigs:
					cancel()
				case <-ticker.C:
					setupLog.Info("validating certs")
					err = crds.CheckCerts(c, dnsName, time.Now().Add(crds.LookaheadInterval+time.Minute))
					if err != nil {
						cancel()
					}
					setupLog.Info("certs are valid")
				}
			}
		}(c, dnsName, certCheckInterval)

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: metricsAddr,
			Port:               9443,
			CertDir:            certDir,
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}
		if err = (&esv1beta1.ExternalSecret{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "ExternalSecret-v1beta1")
			os.Exit(1)
		}
		if err = (&esv1beta1.SecretStore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "SecretStore-v1beta1")
			os.Exit(1)
		}
		if err = (&esv1beta1.ClusterSecretStore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "ClusterSecretStore-v1beta1")
			os.Exit(1)
		}
		if err = (&esv1alpha1.ExternalSecret{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "ExternalSecret-v1alpha1")
			os.Exit(1)
		}
		if err = (&esv1alpha1.SecretStore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "SecretStore-v1alpha1")
			os.Exit(1)
		}
		if err = (&esv1alpha1.ClusterSecretStore{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, errCreateWebhook, "webhook", "ClusterSecretStore-v1alpha1")
			os.Exit(1)
		}
		setupLog.Info("starting manager")
		if err := mgr.Start(ctx); err != nil {
			setupLog.Error(err, "problem running manager")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(webhookCmd)
	webhookCmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	webhookCmd.Flags().StringVar(&dnsName, "dns-name", "localhost", "DNS name to validate certificates with")
	webhookCmd.Flags().StringVar(&certDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "path to check for certs")
	webhookCmd.Flags().StringVar(&loglevel, "loglevel", "info", "loglevel to use, one of: debug, info, warn, error, dpanic, panic, fatal")
	webhookCmd.Flags().DurationVar(&certCheckInterval, "check-interval", 5*time.Minute, "certificate check interval")
}
