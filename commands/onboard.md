---
name: onboard
description: Guided first deploy to Hatch — checks auth, detects your project's framework, builds, deploys, and verifies the live URL.
---

# Hatch onboarding — guided first deploy

Walk the user through their first deploy to Hatch (https://gethatch.eu) end
to end. Work through these steps in order, briefly explaining what you're
doing at each one:

1. **Check readiness.** Call the `get_started` MCP tool. If `authenticated`
   is `false`, call the `login` tool — it opens a browser for sign-in or
   account creation (signup==login). If `login` returns a pending auth URL
   instead of a plan summary, tell the user to sign in in the browser, then
   call `login` again to confirm before continuing.

2. **Detect the project.** Look at the current directory: `package.json`
   (Node — check for a `build` script and a typical output dir like
   `.output`, `.next`, or `dist`), `requirements.txt`/`pyproject.toml`
   (Python), `go.mod` (Go), `Cargo.toml` (Rust), `composer.json` (PHP), or
   plain static HTML with no build step. Call `get_platform_info` if you
   need the exact runtime/base-image mapping.

3. **Build.** Run the project's normal build command (e.g. `npm run build`,
   `go build -o dist/ .`) so a build-output directory is ready to deploy.
   Skip this step for static sites with no build step.

4. **Deploy.** Call `deploy_app` with `deploy_target` (the build output
   directory), `runtime`, and `start_command` (omit for `static`). Let it
   create a new app if the user doesn't already have one for this project.

5. **Verify.** Call `get_status` on the returned app slug and confirm it's
   `running`. If it isn't, use `get_logs` or `get_build_logs` to diagnose
   and retry the deploy.

6. **Hand back the URL.** Tell the user their app is live at the URL
   `deploy_app` returned. Mention `add_domain` if they want a custom domain.

If any step fails, explain the error in plain language and suggest the fix
before moving on — don't just print raw tool output.
