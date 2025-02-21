package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type HelmChartInfo struct {
	ChartName string `json:"chart_name"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
}

type ClusterInfo struct {
	ClusterID   string          `json:"cluster_id"`
	ClusterName string          `json:"cluster_name"`
	KubeVersion string          `json:"kube_version"`
	HelmCharts  []HelmChartInfo `json:"helm_charts"`
}

type HelmRelease struct {
	Chart struct {
		Metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"metadata"`
	} `json:"chart"`
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
}

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create clientset: %v", err)
	}

	clusterName := getClusterName()
	kubeVersion := getKubernetesVersion(clientset)
	clusterID := generateClusterID(clusterName)

	log.Printf("Cluster ID: %s", clusterID)
	log.Printf("Cluster Name: %s", clusterName)
	log.Printf("Kubernetes Version: %s", kubeVersion)

	helmCharts := getLatestHelmReleases(clientset)
	argocdCharts := getArgoCDHelmReleases(clientset)

	helmCharts = append(helmCharts, argocdCharts...)

	output := ClusterInfo{
		ClusterID:   clusterID,
		ClusterName: clusterName,
		KubeVersion: kubeVersion,
		HelmCharts:  helmCharts,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}

	sendDataToAPI(jsonData)
}

func getLatestHelmReleases(clientset *kubernetes.Clientset) []HelmChartInfo {
	var helmCharts []HelmChartInfo

	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to list namespaces: %v", err)
	}

	for _, ns := range namespaces.Items {
		secrets, err := clientset.CoreV1().Secrets(ns.Name).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "owner=helm",
		})
		if err != nil {
			log.Printf("Failed to get secrets in namespace %s: %v", ns.Name, err)
			continue
		}

		for _, secret := range secrets.Items {
			releaseData, ok := secret.Data["release"]
			if !ok {
				continue
			}

			decodedData, err := base64.StdEncoding.DecodeString(string(releaseData))
			if err != nil {
				log.Printf("Failed to decode base64: %v", err)
				continue
			}

			gzReader, err := gzip.NewReader(bytes.NewReader(decodedData))
			if err != nil {
				log.Printf("Failed to create gzip reader: %v", err)
				continue
			}
			defer gzReader.Close()

			var decompressedData bytes.Buffer
			if _, err := io.Copy(&decompressedData, gzReader); err != nil {
				log.Printf("Failed to decompress: %v", err)
				continue
			}

			var helmRelease HelmRelease
			if err := json.Unmarshal(decompressedData.Bytes(), &helmRelease); err != nil {
				log.Printf("Failed to parse JSON: %v", err)
				continue
			}

			if strings.ToLower(helmRelease.Info.Status) != "deployed" {
				continue
			}

			helmCharts = append(helmCharts, HelmChartInfo{
				ChartName: helmRelease.Chart.Metadata.Name,
				Version:   helmRelease.Chart.Metadata.Version,
				Namespace: ns.Name,
			})
		}
	}

	return helmCharts
}

func getArgoCDHelmReleases(clientset *kubernetes.Clientset) []HelmChartInfo {
	var helmCharts []HelmChartInfo

	//label "argocd.argoproj.io/instance"
	deployments, err := clientset.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "argocd.argoproj.io/instance",
	})
	if err != nil {
		log.Printf("Failed to get ArgoCD-managed deployments: %v", err)
		return helmCharts
	}

	for _, deploy := range deployments.Items {
		deploymentName := deploy.Name
		namespace := deploy.Namespace
		version := "unknown"

		for key, value := range deploy.Labels {
			if strings.Contains(strings.ToLower(key), "chart") {
				version = value
				break
			}
		}

		helmCharts = append(helmCharts, HelmChartInfo{
			ChartName: deploymentName,
			Version:   version,
			Namespace: namespace,
		})
	}

	return helmCharts
}

func sendDataToAPI(jsonData []byte) {
	apiURL := os.Getenv("API_URL")
	apiToken := os.Getenv("API_TOKEN")

	if apiURL == "" || apiToken == "" {
		log.Println("API_URL or API_TOKEN not set, skipping API request")
		return
	}

	req, err := http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-token", apiToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send data to API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Println("Successfully sent data to API")
	} else {
		log.Printf("API request failed with status: %d", resp.StatusCode)
	}
}

func generateClusterID(clusterName string) string {
	namespaceUUID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(clusterName))
	return namespaceUUID.String()
}

func getClusterName() string {
	if envClusterName := os.Getenv("CLUSTER_NAME"); envClusterName != "" {
		return envClusterName
	}
	return "unknown-cluster"
}

func getKubernetesVersion(clientset *kubernetes.Clientset) string {
	versionInfo, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "unknown-version"
	}
	return versionInfo.GitVersion
}
