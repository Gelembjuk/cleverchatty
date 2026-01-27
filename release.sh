#!/bin/bash
set -e

if [ -z "$1" ]; then
    echo "Usage: ./release.sh <version>"
    echo "Example: ./release.sh 0.4.1"
    exit 1
fi

VERSION="$1"
MODULE_BASE="github.com/gelembjuk/cleverchatty"
REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "=== Releasing v${VERSION} ==="

# Ensure working tree is clean
if [ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]; then
    echo "Error: working tree is not clean. Commit or stash changes first."
    exit 1
fi

# Step 1: Tag and push core
echo ""
echo "--- Tagging core/v${VERSION} ---"
git -C "$REPO_ROOT" tag "core/v${VERSION}"
git -C "$REPO_ROOT" push origin "core/v${VERSION}"

# Step 2: Wait for Go module proxy to index the new core tag
echo ""
echo "--- Waiting for Go module proxy to index core/v${VERSION} ---"
PROXY_URL="https://proxy.golang.org/${MODULE_BASE}/core/@v/v${VERSION}.info"
for i in $(seq 1 30); do
    if curl -sf "$PROXY_URL" > /dev/null 2>&1; then
        echo "Module proxy has indexed core/v${VERSION}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "Warning: timed out waiting for proxy. You may need to finish manually."
        exit 1
    fi
    echo "  waiting... (attempt $i/30)"
    sleep 5
done

# Step 3: Update go.mod in dependent modules
for mod in cleverchatty-cli cleverchatty-server; do
    echo ""
    echo "--- Updating ${mod}/go.mod to use core v${VERSION} ---"
    cd "$REPO_ROOT/$mod"
    GOWORK=off go get "${MODULE_BASE}/core@v${VERSION}"
    GOWORK=off go mod tidy
done

# Step 4: Commit updated go.mod/go.sum files
cd "$REPO_ROOT"
git add cleverchatty-cli/go.mod cleverchatty-cli/go.sum \
        cleverchatty-server/go.mod cleverchatty-server/go.sum
if git diff --cached --quiet; then
    echo "No go.mod changes needed."
else
    git commit -m "Update core dependency to v${VERSION}"
    git push
fi

# Step 5: Tag and push the dependent modules
for mod in cleverchatty-cli cleverchatty-server; do
    echo ""
    echo "--- Tagging ${mod}/v${VERSION} ---"
    git tag "${mod}/v${VERSION}"
    git push origin "${mod}/v${VERSION}"
done

echo ""
echo "=== Release v${VERSION} complete ==="
echo "Users can now install with:"
echo "  go install ${MODULE_BASE}/cleverchatty-cli@v${VERSION}"
echo "  go install ${MODULE_BASE}/cleverchatty-server@v${VERSION}"
