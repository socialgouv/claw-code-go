#!/usr/bin/env bash
# sync_bundled_skills.sh — resync internal/tools/bundled_skills/ from a local
# clone of anthropics/claude-plugins-official. Preserves the marketplace
# structure <plugin>/skills/<skill>/ (does NOT flatten).
#
# Usage:
#   scripts/sync_bundled_skills.sh [marketplace_root]
#
# Defaults marketplace_root to:
#   ~/.claude/plugins/marketplaces/claude-plugins-official
#
# Idempotent: wipes the target subdirectory of each listed skill first.
set -euo pipefail

MARKETPLACE="${1:-$HOME/.claude/plugins/marketplaces/claude-plugins-official}"
DEST="$(cd "$(dirname "$0")/.." && pwd)/internal/tools/bundled_skills"

if [ ! -d "$MARKETPLACE/plugins" ]; then
  echo "marketplace not found: $MARKETPLACE" >&2
  echo "clone https://github.com/anthropics/claude-plugins-official.git first" >&2
  exit 1
fi

# Canonical "plugin:skill" list — 18 skills.
SKILLS=(
  "plugin-dev:agent-development"
  "plugin-dev:command-development"
  "plugin-dev:hook-development"
  "plugin-dev:mcp-integration"
  "plugin-dev:plugin-settings"
  "plugin-dev:plugin-structure"
  "plugin-dev:skill-development"
  "mcp-server-dev:build-mcp-app"
  "mcp-server-dev:build-mcp-server"
  "mcp-server-dev:build-mcpb"
  "claude-code-setup:claude-automation-recommender"
  "claude-md-management:claude-md-improver"
  "frontend-design:frontend-design"
  "hookify:writing-rules"
  "math-olympiad:math-olympiad"
  "playground:playground"
  "session-report:session-report"
  "skill-creator:skill-creator"
)

echo "Syncing ${#SKILLS[@]} skills from $MARKETPLACE → $DEST"

for entry in "${SKILLS[@]}"; do
  plugin="${entry%%:*}"
  skill="${entry##*:}"
  src="$MARKETPLACE/plugins/$plugin/skills/$skill"
  dst="$DEST/$plugin/skills/$skill"

  if [ ! -d "$src" ]; then
    echo "  skip $entry (missing source: $src)" >&2
    continue
  fi

  rm -rf "$dst"
  mkdir -p "$(dirname "$dst")"
  cp -R "$src" "$dst"
  # Drop noisy files that don't belong in an embedded asset.
  find "$dst" -type d -name "__pycache__" -prune -exec rm -rf {} + 2>/dev/null || true
  find "$dst" -type d -name ".git" -prune -exec rm -rf {} + 2>/dev/null || true
  echo "  ok   $entry"
done

echo "Done. Commit internal/tools/bundled_skills/."
