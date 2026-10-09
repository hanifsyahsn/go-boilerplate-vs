#!/bin/bash
set -e

export VERSION=$(cat VERSION)
echo "📦 Building Docker image for version $VERSION..."

docker compose build --build-arg VERSION=$VERSION app

echo "🧹 Cleaning up dangling images from this project..."
DANGLING=$(docker images -f "dangling=true" -f "label=project=go_boilerplate_vs_api_label" -q)

if [ -n "$DANGLING" ]; then
    echo "Found dangling: $DANGLING"
    docker rmi $DANGLING
else
    echo "ℹ️  No dangling images to clean up."
fi

echo "✅ Build complete: go_boilerplate_vs_api:${VERSION}"
