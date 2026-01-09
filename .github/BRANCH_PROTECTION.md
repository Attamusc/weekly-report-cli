# Branch Protection Recommendations

This document outlines recommended branch protection rules for this repository to ensure code quality and security.

## Recommended Settings for `main` branch

### Required Status Checks

Enable "Require status checks to pass before merging" with the following checks:

- **CI / Lint** - Ensures code passes linting standards
- **CI / Test** - Ensures all tests pass with race detection
- **CI / Build** - Ensures code compiles successfully

Configure:
- ✅ Require branches to be up to date before merging
- ✅ Do not require status checks on creation (allows initial setup)

### Pull Request Reviews

- ✅ Require pull request reviews before merging
  - Required approving reviews: **1** (adjust based on team size)
  - ✅ Dismiss stale pull request approvals when new commits are pushed
  - ✅ Require review from Code Owners (if CODEOWNERS file is added)

### Additional Protections

- ✅ Require conversation resolution before merging
- ✅ Require linear history (prevents merge commits, enforces rebase/squash)
- ✅ Do not allow bypassing the above settings (even for administrators)

### Recommended but Optional

- ⚠️ Require signed commits (requires all contributors to set up GPG signing)
- ⚠️ Include administrators (recommended for consistency, but may slow down urgent fixes)

## Security Workflow

The Security workflow runs weekly and on pull requests but does **not** block merges:
- Provides visibility into vulnerabilities
- Allows the team to address issues without blocking development
- Results available in Security tab (SARIF reports)

## How to Apply These Settings

### Via GitHub UI

1. Go to your repository Settings
2. Navigate to "Branches" under "Code and automation"
3. Click "Add rule" for the `main` branch
4. Apply the settings listed above
5. Click "Create" or "Save changes"

### Via GitHub CLI

```bash
# Requires gh CLI tool and repo admin permissions
gh api repos/OWNER/REPO/branches/main/protection -X PUT --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "CI / Lint",
      "CI / Test", 
      "CI / Build"
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismissal_restrictions": {},
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true
}
EOF
```

Replace `OWNER/REPO` with your repository path (e.g., `Attamusc/weekly-report-cli`).

## Testing the Setup

After applying branch protection rules:

1. Create a test branch: `git checkout -b test/branch-protection`
2. Make a small change and push
3. Open a pull request
4. Verify all CI checks run automatically
5. Confirm you cannot merge until checks pass
6. Test that force pushes are blocked
7. Clean up the test branch after verification

## Notes

- Security scans run but don't block PRs - review them regularly
- Adjust review requirements based on team size and workflow
- Consider adding a CODEOWNERS file for automatic review assignments
- Re-evaluate these settings as the project and team grow
