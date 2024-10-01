package helper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

type ApiEndpointTest struct {
	BearerTokenRetrievalFn func(baseUrl string) (string, error)
	Endpoint               string
	ExpectedHttpStatusCode int
	BodyMatcher            types.GomegaMatcher
}

// BackstageAuthRefreshResponse is the struct of the response returned by the '/api/auth/:user/refresh' API endpoint.
//
// Example:
//
//	{
//	   "backstageIdentity": {
//	       "expiresInSeconds": 3600,
//	       "identity": {
//	           "ownershipEntityRefs": [
//	               "user:development/guest"
//	           ],
//	           "type": "user",
//	           "userEntityRef": "user:development/guest"
//	       },
//	       "token": "eyJ0..."
//	   },
//	   "profile": {}
//	}
type BackstageAuthRefreshResponse struct {
	BackstageIdentity BackstageIdentity `json:"backstageIdentity,omitempty"`
}

type BackstageIdentity struct {
	Token string `json:"token,omitempty"`
}

func VerifyBackstagePodStatus(g Gomega, ns string, crName string, expectedStatus string) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-l", "rhdh.redhat.com/app=backstage-"+crName,
		"-o", "jsonpath={.items[*].status}",
		"-n", ns,
	) // #nosec G204
	status, err := Run(cmd)
	fmt.Fprintln(GinkgoWriter, string(status))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(ContainSubstring(fmt.Sprintf(`"phase":%q`, expectedStatus)),
		fmt.Sprintf("backstage pod in %s status", status))
}

func VerifyBackstageCRStatus(g Gomega, ns string, crName string, expectedStatus string) {
	cmd := exec.Command(GetPlatformTool(), "get", "backstage", crName, "-o", "jsonpath={.status.conditions}", "-n", ns) // #nosec G204
	status, err := Run(cmd)
	fmt.Fprintln(GinkgoWriter, string(status))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(ContainSubstring(expectedStatus),
		fmt.Sprintf("status condition with type %s should be set", expectedStatus))
}

func PatchBackstageCR(ns string, crName string, jsonPatch string, patchType string) error {
	p := patchType
	if p == "" {
		p = "strategic"
	}
	_, err := Run(exec.Command(GetPlatformTool(), "-n", ns, "patch", "backstage", crName, "--patch", jsonPatch, "--type="+p)) // #nosec G204
	return err
}

func DoesBackstageRouteExist(ns string, crName string) (bool, error) {
	routeName := model.RouteName(crName)
	out, err := Run(exec.Command(GetPlatformTool(), "get", "route", routeName, "-n", ns)) // #nosec G204
	if err != nil {
		if strings.Contains(string(out), fmt.Sprintf("%q not found", routeName)) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func GetBackstageRouteHost(ns string, crName string) (string, error) {
	routeName := model.RouteName(crName)

	hostBytes, err := Run(exec.Command(
		GetPlatformTool(), "get", "route", routeName, "-o", "go-template={{if .spec.host}}{{.spec.host}}{{end}}", "-n", ns)) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine host for route %s/%s: %w", ns, routeName, err)
	}
	host := string(hostBytes)
	if host != "" {
		return host, nil
	}

	// try with subdomain in case it was set
	subDomainBytes, err := Run(exec.Command(
		GetPlatformTool(), "get", "route", routeName, "-o", "go-template={{if .spec.subdomain}}{{.spec.subdomain}}{{end}}", "-n", ns)) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine subdomain for route %s/%s: %w", ns, routeName, err)
	}
	subDomain := string(subDomainBytes)
	if subDomain == "" {
		return "", nil
	}
	ingressDomainBytes, err := Run(exec.Command(GetPlatformTool(), "get", "ingresses.config/cluster", "-o", "jsonpath={.spec.domain}")) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine ingress sub-domain: %w", err)
	}
	ingressDomain := string(ingressDomainBytes)
	if ingressDomain == "" {
		return "", nil
	}
	return fmt.Sprintf("%s.%s", subDomain, ingressDomain), err
}

// unauthenticated endpoints
var defaultApiEndpointTests = []ApiEndpointTest{
	{
		Endpoint:               "/",
		ExpectedHttpStatusCode: 200,
		BodyMatcher:            ContainSubstring("You need to enable JavaScript to run this app"),
	},
}

func VerifyBackstageRoute(g Gomega, ns string, crName string, tests []ApiEndpointTest) {
	host, err := GetBackstageRouteHost(ns, crName)
	fmt.Fprintln(GinkgoWriter, host)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(host).ShouldNot(BeEmpty())
	baseUrl := fmt.Sprintf("https://%s", host)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- test code only, not used in production
		},
	}
	httpClient := &http.Client{Transport: tr}

	performTest := func(tt ApiEndpointTest) {
		url := fmt.Sprintf("%s/%s", baseUrl, strings.TrimPrefix(tt.Endpoint, "/"))

		req, reqErr := http.NewRequest("GET", url, nil)
		g.Expect(reqErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while building request to GET %q", url))

		req.Header.Add("Accept", "application/json")

		if tt.BearerTokenRetrievalFn != nil {
			bearerToken, tErr := tt.BearerTokenRetrievalFn(baseUrl)
			g.Expect(tErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while retrieving bearer token, context: %q", tt.Endpoint))
			if bearerToken != "" {
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", bearerToken))
			}
		}

		fmt.Fprintf(GinkgoWriter, "--> GET %q\n", url)
		resp, rErr := httpClient.Do(req)
		g.Expect(rErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while trying to GET %q", url))
		defer resp.Body.Close()
		body, rErr := io.ReadAll(resp.Body)
		g.Expect(rErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while trying to read response body from 'GET %q'", url))
		bodyStr := string(body)
		g.Expect(resp.StatusCode).Should(Equal(tt.ExpectedHttpStatusCode), fmt.Sprintf("context: %s\n===Response body===\n%s", tt.Endpoint, bodyStr))
		if tt.BodyMatcher != nil {
			g.Expect(bodyStr).Should(tt.BodyMatcher, "context: "+tt.Endpoint)
		}
	}
	allTests := append(defaultApiEndpointTests, tests...)
	for _, tt := range allTests {
		performTest(tt)
	}
}
