#!/bin/bash

echo "Patching services with OTEL configurations ..."
echo "================================================================================"
kubectl config set-context --current --namespace=blog-apps

BLOG_SERVICE_OTEL_CONF='{"spec": {"template": {"metadata": {"annotations": {"instrumentation.opentelemetry.io/inject-go": "opentelemetry-operator-system/otel-instrumentation", "instrumentation.opentelemetry.io/otel-go-auto-target-exe": "/app/src/blog-service/app"}}}}}'
TEXT_ANALYZE_SERVICE_OTEL_CONF='{"spec": {"template": {"metadata": {"annotations": {"instrumentation.opentelemetry.io/inject-go": "opentelemetry-operator-system/otel-instrumentation", "instrumentation.opentelemetry.io/otel-go-auto-target-exe": "/app/src/text-analyze-service/app"}}}}}'

kubectl patch deployment.apps/blog-service -p "$BLOG_SERVICE_OTEL_CONF"
kubectl patch deployment.apps/text-analyze-service -p "$TEXT_ANALYZE_SERVICE_OTEL_CONF"
echo "================================================================================"
echo "Patched services with OTEL configurations"

echo "Waiting for the services to be ready ..."
sleep 20
echo "Services are ready now. Visit http://localhost:30000"
