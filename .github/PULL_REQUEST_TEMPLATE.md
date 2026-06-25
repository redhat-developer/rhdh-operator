<!-- 
Thank you for opening a PR! Please take the time to fill in the details below.
-->

## Description
<!--
Please explain the changes you made here.
-->

## Which issue(s) does this PR fix or relate to

- Fixes #_issue_number_

## PR acceptance criteria

- [ ] Tests
- [ ] Documentation

## How to test changes / Special notes to the reviewer
<!--
Detailed instructions may help reviewers test this PR quickly and provide quicker feedback.
-->

## Building Container Images for Testing

Need to test container images from this PR?

**For Maintainers:** Triggering Builds
To trigger a test image build, review the code and comment with the specific commit SHA you are approving:
`/build-images <sha>` *(e.g., `/build-images a1b2c3d`)*

*(You can find the short SHA at the bottom of the PR timeline or in the Commits tab).*

**For Contributors:** Ask a maintainer to run `/build-images <sha>`

Images will be built and pushed to Quay with links posted in comments.
