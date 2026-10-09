#!/bin/bash
set -e

export VERSION=$(cat VERSION)
CONTAINER_NAME="go_boilerplate_vs_api"
IMAGE_TAG="go_boilerplate_vs_api:${VERSION}"
TIMEOUT=30

echo "🚀 Deploying version $VERSION..."

if ! docker image inspect "$IMAGE_TAG" >/dev/null 2>&1; then
    echo "❌ Image $IMAGE_TAG not found. Run build.sh first."
    exit 1
fi

if docker ps -q --filter "name=$CONTAINER_NAME" --filter "status=running" | grep -q .; then
    echo "⏳ Sending SIGTERM to $CONTAINER_NAME..."
    docker kill --signal=SIGTERM $CONTAINER_NAME

    echo "⏳ Waiting for container to finish (timeout: ${TIMEOUT}s)..."
    ELAPSED=0
    while docker ps -q --filter "name=$CONTAINER_NAME" --filter "status=running" | grep -q .; do
        if [ $ELAPSED -ge $TIMEOUT ]; then
            echo "⚠️  Timeout reached, force stopping container..."
            docker compose stop app
            break
        fi
        echo "ℹ️ still running... ($ELAPSED s)"
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    done
    echo "✅ Old container stopped."
else
    echo "ℹ️ No running container found, skipping graceful shutdown..."
fi

echo "🚀 Starting new version $VERSION..."
docker compose up -d app

echo "🎉 Deployed version $VERSION successfully!"
