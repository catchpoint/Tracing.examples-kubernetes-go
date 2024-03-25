#!/bin/bash

echo "Deleting services ..."
echo "================================================================================"
kubectl delete service text-analyze-svc
kubectl delete service blog-svc
echo "================================================================================"
echo "Deleted services"

echo "Deleting deployments ..."
echo "================================================================================"
kubectl delete deployment blog-service
kubectl delete deployment text-analyze-service
echo "================================================================================"
echo "Deleted deployments"

echo "Deleting namespace ..."
echo "================================================================================"
kubectl delete namespace blog-apps
echo "================================================================================"
echo "Deleted namespace"
echo "All resources have been deleted"
