# CI/CD Integration

CIDX acts as a universal runner, meaning your CI/CD configuration becomes very simple. It primarily needs to install CIDX and then call `cidx run <pipeline>`.

## GitHub Actions

### Basic Workflow

This workflow runs on every push and pull request.

```yaml
name: CI

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install CIDX
        run: go install github.com/cidx-org/cidx/cmd/cidx@latest

      - name: Run CI Pipeline
        run: cidx run ci
```

### Release Workflow

CIDX can also handle your release process, ensuring that what you test locally is exactly what runs in production.

```yaml
release:
  needs: [validate]
  if: startsWith(github.ref, 'refs/tags/v')
  permissions:
    contents: write
  steps:
    - uses: actions/checkout@v4

    - name: Install CIDX
      run: go install github.com/cidx-org/cidx/cmd/cidx@latest

    - name: Create Release
      run: cidx run gh-release
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        TAG: ${{ github.ref_name }}
```

## GitLab CI

```yaml
stages:
  - ci

cidx:
  stage: ci
  image: golang:latest
  script:
    - go install github.com/cidx-org/cidx/cmd/cidx@latest
    - cidx run ci
  services:
    - docker:dind
  variables:
    DOCKER_HOST: tcp://docker:2375
```

## Jenkins

```groovy
pipeline {
    agent any
    stages {
        stage('CI') {
            steps {
                sh 'go install github.com/cidx-org/cidx/cmd/cidx@latest'
                sh 'cidx run ci'
            }
        }
    }
}
```
