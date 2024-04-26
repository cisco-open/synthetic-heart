# How to Contribute

Thank you for considering contributing to synthetic-heart. With everyone's contribution we can make synthetic-heart a great addition to the Kubernetes ecosystem.

Following these guidelines helps to communicate that you respect the time of the developers managing and developing this open source project. In return, they should reciprocate that respect in addressing your issue, assessing changes, and helping you finalize your pull requests. In that spirit of mutual respect,
we endeavor to review incoming issues and pull requests within 10 days, and will close any lingering issues or pull requests after 60 days of inactivity.

synthetic-heart is an open source project and we love to receive contributions from our community — you! There are many ways to contribute, from writing tutorials or blog posts, improving the documentation, submitting bug reports and feature requests or writing code which can be incorporated into synthetic-heart itself.

Please note that all of your interactions in the project are subject to our [Code of Conduct](/CODE_OF_CONDUCT.md). This
includes creation of issues or pull requests, commenting on issues or pull requests, and extends to all interactions in
any real-time space e.g., Slack, Discord, etc.

## Code Contribution

### Setting up a build environment

Prerequisites:
  - A MacOS or Linux dev machine. 
  - [Podman](https://podman-desktop.io/)
  - A Kubernetes cluster (Tested with v1.25+, older versions may work)
    - You can use [Kind](https://kind.sigs.k8s.io/) to create a local cluster. Follow the [Podman doc page](https://podman-desktop.io/docs/kind/creating-a-kind-cluster) to see how.
  - [Go 1.22](https://go.dev/doc/install)
  - [Helm v3](https://helm.sh/docs/intro/install/#helm)
  - [Make](https://www.gnu.org/software/make/)

Once you have a cluster to develop on, install the Helm chart to that cluster, as described in the [README](./README.md). Following that, the workflow would look something like this:

  - Make code changes.
  - Run `make docker-all`.
    - This compiles all the code and creates container images for agent, controller and restapi with tag `dev_latest`.
  - Push the latest image to the Kind cluster. Can be done through the Podman desktop UI. [Link to instructions](https://podman-desktop.io/docs/kubernetes/kind/pushing-an-image-to-kind).
  - Restart the relevant pods, so the pod starts with the new image.

Note: The Controller has special commands to make changes to CRD files etc. Please refer to the [controller README](./controller/README.md).

Note 2: The Agent also has special commands to make changes to proto files. Please refer to the [agent README](./agent/README.md).


### Writing new synthetic test plugins for the agent

Synthetic-heart welcomes the community to contribute new plugins for the agents. Please refer to the [agent README](./agent/README.md) to see how to write new plugins.

## Reporting Issues

Before reporting a new issue, please ensure that the issue was not already reported or fixed by searching through our
[issues list](https://github.com/cisco-open/synthetic-heart/issues).

When creating a new issue, please be sure to include a **title and clear description**, as much relevant information as
possible, and, if possible, a test case.

**If you discover a security bug, please do not report it through GitHub. Instead, please see security procedures in
[SECURITY.md](/SECURITY.md).**

## Sending Pull Requests

Before sending a new pull request, take a look at existing pull requests and issues to see if the proposed change or fix
has been discussed in the past, or if the change was already implemented but not yet released.

We expect new pull requests to include tests for any affected behavior, and, as we follow semantic versioning, we may
reserve breaking changes until the next major version release.

## Other Ways to Contribute

We welcome anyone that wants to contribute to `synthetic-heart` to triage and reply to open issues to help troubleshoot
and fix existing bugs. Here is what you can do:

- Help ensure that existing issues follows the recommendations from the _[Reporting Issues](#reporting-issues)_ section,
  providing feedback to the issue's author on what might be missing.
- Review existing pull requests, and testing patches against real existing applications that use `synthetic-heart`.
- Write a test, or add a missing test case to an existing test.

Thanks again for your interest on contributing to `synthetic-heart`!

:heart:
