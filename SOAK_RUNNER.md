# Running Soak Tests

The test framework is designed around you launching a test environment to a K8s cluster, then running the test from your personal machine. Your personal machine coordinates the chainlink nodes, reads the blockchain, etc. This works fine for running tests that take < 5 minutes, but soak tests often last days or weeks. So the tests become dependent on your local machine maintaining power and network connection for that time frame. This quickly becomes untenable.

So the solution is to launch a `remote-test-runner` container along with the test environment. This runner pulls the project from a specified repo and branch, builds it, then executes the test from within the K8s namespace. **This means that if you want changes in how the test runs, you need to push these changes to a public repo!**

## Set Test Params

Before building anything, you need to set the parameters for the test to run on. Right now, there's only the OCR soak test, so you can head over to the [ocr soak test file](./suite/soak/tests/ocr_test.go) and check out the `Setup` step, where you can set a few different inputs that changes how the test runs.

## Set Email Notification (optional)

The [framework.yaml file](./framework.yaml) holds a section where you can define an email address to use to notify you when the test is done. The email will come from your own email account, detailing if the test passed or failed, and providing some summary information. This is still a bit of an experimental feature, so check in on the test running occasionally as well.

```yaml
# Experimental: Let the remote test runner email you when the test is done.
remote_runner_email_server: smtp.gmail.com # Example if you use GMAIL
remote_runner_email_address: # Username or email address to use
remote_runner_email_pass: # Password to email account. REMEMBER TO DELETE
```

If using GMail with 2-factor auth, you need to enable an [app password](https://support.google.com/accounts/answer/185833?p=InvalidSecondFactor&visit_id=637814253495195963-1201613062&rd=1) for this to work. I have not tried it with other email clients, so let me know if you have issues.

## Run The Test

See the [soak runner file](./suite/soak/soak_runner_test.go) that will actually trigger these tests. Right now there is only OCR, but you can easily add more triggers by following that example. You can use IDE features to launch the tests from there, or run `make test_soak`. It will do the following:

1. Launch a specified chainlink environment
2. Wait for that environment to come up as healthy
3. Launch a `remote-test-runner` that pulls from specified repo and branch
4. Wait for that test runner to come up as healthy

You can change the behavior of what tests to use and run by modifying the code below, then running the `soak_runner_test.go` file.

```go
env, err := environment.DeployLongTestEnvironment(
  environment.NewLongRunningChainlinkConfig(environment.ChainlinkReplicas(6, nil)),
  "git://github.com/smartcontractkit/integrations-framework.git", // The repo to pull your tests from
  "main",               // The branch of the above repo to pull tests from
  "./suite/soak/tests", // The path of test files
  "@soak-ocr",          // REGEX name of the specific test you want to run
  config.ProjectFrameworkSettings.RemoteRunnerEmailServer,
  config.ProjectFrameworkSettings.RemoteRunnerEmailAddress,
  config.ProjectFrameworkSettings.RemoteRunnerEmailPass,
  tools.ChartsRoot,
)
```

## Watching the Test

The test environment **will stay active until you manually delete it from your Kubernetes cluster**. This keeps the test env alive so you can view the logs when the test is done. 
You can do so by [using kubectl](https://www.dnsstuff.com/how-to-tail-kubernetes-and-kubectl-logs), something like [Lens](https://k8slens.dev/), or use the `chainlink-soak-xxyyx.yaml` as an environment file with our handy [helmenv](https://github.com/smartcontractkit/helmenv) tool.

Using the `helmenv` cli tool, you can input the generated file to download logs like so.

`envcli dump -e chainlink-long-xxyyx.yaml -a soak_test_logs -db chainlink`
