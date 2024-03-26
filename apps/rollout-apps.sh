#!/bin/bash

echo "Rolling out services ..."
echo "================================================================================"
kubectl rollout restart deployment text-analyze-service --namespace=blog-apps
kubectl rollout restart deployment blog-service --namespace=blog-apps
echo "================================================================================"
echo "Rolled out services"

echo "Waiting for the services to be ready ..."
sleep 20
echo "Services are ready now. Visit http://localhost:30000"
