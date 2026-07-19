# Mission: Open the interactive picker for an existing empty config

## M1: Runtime diagnosis | status: in_progress
### T1.1: Reproduce and isolate | agent:Reviewer
- [ ] S1.1.1: Reproduce the reported failure twice with an existing empty template and a known-good fake SSH config | size:S
- [ ] S1.1.2: Toggle missing-vs-existing config state to confirm the control-flow root cause and record evidence | size:S

## M2: Regression-safe fix | depends:M1
### T2.1: Typed configuration state | agent:Worker
- [ ] S2.1.1: Add a failing test that distinguishes the no-server config state without matching error text | size:S
- [ ] S2.1.2: Introduce a sentinel no-server error wrapped with `%w` and make the RED test pass | size:S

### T2.2: Existing-template population | agent:Worker | depends:T2.1
- [ ] S2.2.1: Add a failing test that populates an existing empty config while preserving interval, thresholds, and LLM settings | size:M
- [ ] S2.2.2: Implement a secure atomic update limited to a known no-server config and route interactive startup through the picker | size:M
- [ ] S2.2.3: Preserve missing-config and headless behavior | size:S

## M3: Verification | depends:M2
### T3.1: Automated gates | agent:Reviewer
- [ ] S3.1.1: Run gofmt plus `go build ./...` and `go vet ./...` successfully | size:S
- [ ] S3.1.2: Run `go test -race -shuffle=on -count=1 ./...` successfully | size:S
- [ ] S3.1.3: Verify `git diff --check` and intended bugfix diff are clean | size:S

### T3.2: User-scenario QA | agent:Reviewer
- [ ] S3.2.1: Run a real pseudo-terminal smoke from an existing empty config and observe the picker | size:M
- [ ] S3.2.2: Select an included and ungrouped host, verify settings/group preservation, and observe immediate TUI launch | size:M

## M4: Cleanup and delivery | depends:M3
### T4.1: Debug artifact cleanup | agent:Reviewer
- [ ] S4.1.1: Remove every journaled debug artifact and temporary exclusion, leaving only fix/test/docs changes | size:S

### T4.2: Git and final system verification | agent:Reviewer
- [ ] S4.2.1: Create and verify one atomic Russian Conventional Commit for the bugfix | size:S
- [ ] S4.2.2: Re-run committed-tree gates and confirm no unresolved sync issues | size:M
