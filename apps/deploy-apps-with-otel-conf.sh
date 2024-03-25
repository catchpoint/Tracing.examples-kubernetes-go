#!/bin/bash
echo "Creating Docker image ..."
echo "================================================================================"
docker build -f apps/Dockerfile -t blog-app .
echo "================================================================================"
echo "Created Docker image"

echo "Creating and activating application namespace ..."
echo "================================================================================"
kubectl create namespace blog-apps
kubectl config set-context --current --namespace=blog-apps
echo "================================================================================"
echo "Created and activating application namespace"

echo "Creating deployment with OTEL configurations ..."
echo "================================================================================"
kubectl apply -f apps/deployment-with-otel-conf.yaml
echo "================================================================================"
echo "Created deployment with OTEL configurations"

echo "Waiting for the services to be ready ..."
sleep 20
echo "Services are ready now. Visit http://localhost:30000"
