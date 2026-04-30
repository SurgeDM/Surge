#!/bin/bash

# This script installs the git hooks for the Surge project.

HOOKS_DIR=".git/hooks"
SOURCE_HOOK="scripts/pre-push"

if [ ! -d ".git" ]; then
    echo "Error: .git directory not found. Are you in the root of the Surge project?"
    exit 1
fi

echo "Installing git hooks..."

# Copy pre-push hook
cp "$SOURCE_HOOK" "$HOOKS_DIR/pre-push"
chmod +x "$HOOKS_DIR/pre-push"

echo "Hooks installed successfully!"
