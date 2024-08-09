#! /bin/bash
# prerequisite `make build-dorifi`

export API_SERVER_ROOT="https://cf-api.korifi.cki.shoot.canary.k8s-hana.ondemand.com"
export APP_FQDN="apps.korifi.cki.shoot.canary.k8s-hana.ondemand.com"
export NO_PARALLEL=false

go run github.com/onsi/ginkgo/v2/ginkgo --output-interceptor-mode=none --randomize-all --randomize-suites tests/smoke