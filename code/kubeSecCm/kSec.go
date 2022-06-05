/*
Read string secret from AzureKV and create kubernetes secret object

Purpose is to use in initContainer to setup a secrets for Redis Cluster with authpass stored in AzureKV, pod need to have managedIdentity enabled to access KV api

service account need to has role for list and create secrets in namespace which is not by default
kubectl create role access-secrets --verb=get,list,watch,update,create --resource=secrets
kubectl create rolebinding --role=access-secrets default-to-secrets --serviceaccount=podkova:default
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azsecrets "github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2 "k8s.io/client-go/applyconfigurations/core/v1"
	metaappv1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func getSecretFromKV(keyVaultName string, secretName []string) map[string]string {
	secMap := make(map[string]string)
	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	// cred, err := azidentity.NewAzureCLICredential(nil)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatalf("failed to obtain a credential: %v", err)
	}

	cli, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		log.Fatalf("failed to create a client: %v", err)
	}

	for l := 0; l < len(secretName); l++ {
		resp, err := cli.GetSecret(context.TODO(), secretName[l], nil)
		if err != nil {
			log.Fatalf("failed to read a secret: %v", err)
		}

		secMap[*resp.Name] = *resp.Value

		// fmt.Printf("Name: %s, Value: %s\n", *resp.Name, *resp.Value)
	}
	return secMap
}

func initClient() *kubernetes.Clientset {
	// kubeconfig := os.Getenv("HOME") + "/.kube/opsdemo"
	// config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	setclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return setclient

}

func createSecret(sname string, namespace string, content map[string]string) *v1.Secret {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sname,
			Namespace: namespace,
		},
		StringData: content,
	}
	return secret
}

func isSecretThere(setclient *kubernetes.Clientset, sname string, namespace string) (action string) {
	obj, err := setclient.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		// fmt.Printf("%#v\n", err)
		panic(err.Error())
	}
	if len(obj.Items) == 0 {
		action = "create"
		return action
	}
	for _, v := range obj.Items {
		// fmt.Printf("%v\n", v.Name)
		if v.Name == sname {
			action = "replace"
			return action
		} else {
			action = "create"
		}
	}
	return action
}

func applySecret(file string, cmname string, namespace string, content map[string]string) *v2.SecretApplyConfiguration {
	secret := &v2.SecretApplyConfiguration{
		ObjectMetaApplyConfiguration: &metaappv1.ObjectMetaApplyConfiguration{
			Name:      &cmname,
			Namespace: &namespace,
		},
		StringData: content,
	}

	// fmt.Printf("%+v", secret)
	return secret
}

// func envVariables(env map[string]string) map[string]string {
// 	envmap := make(map[string]string)
// 	for key, value := range env {
// 		key, ok := os.LookupEnv(value)
// 		if !ok {
// 			log.Default().Fatalf("env variable %s not found", value)
// 		} else {
// 			envmap[
// 		}
// 	}
// }

func main() {
	/*
		export KV_NAME=honeypotvault
		export KV_SECRET=master
		export POD_NAMESPACE=podkova
		export SECRET_NAME=helma1
	*/
	kvName, ok := os.LookupEnv("KV_NAME")
	if !ok {
		log.Default().Fatalf("env variable %s not found", "KV_NAME")
		fmt.Printf("KV_NAME: %s\n", kvName)
	} else {
		fmt.Printf("KV_NAME: %s\n", kvName)
	}

	secretNameInKV, ok := os.LookupEnv("KV_SECRET")
	if !ok {
		log.Default().Fatalf("env variable %s not found", "KV_SECRET")

	} else {
		fmt.Printf("KV_SECRET: %s\n", secretNameInKV)
	}

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		log.Default().Fatalf("env variable %s not found", "POD_NAMESPACE")
	} else {
		fmt.Printf("POD_NAMESPACE: %s\n", namespace)
	}

	secretName, ok := os.LookupEnv("SECRET_NAME")
	if !ok {
		log.Default().Fatalf("env variable %s not found", "SECRET_NAME")
	} else {
		fmt.Printf("SECRET_NAME: %s\n", secretName)
	}

	kvSecret := getSecretFromKV(kvName, []string{secretNameInKV})

	//set connection

	setclient := initClient()
	// check if secret is present
	action := isSecretThere(setclient, secretName, namespace)
	switch action {
	//create new secret
	case "create":
		model := createSecret(secretName, namespace, kvSecret)
		_, err := setclient.CoreV1().Secrets(namespace).Create(context.TODO(), model, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("%+v", err)
		} else {
			fmt.Printf("Secret %s created\n", secretName)
		}
		//update present secret
	case "replace":
		cmmodel := createSecret(secretName, namespace, kvSecret)
		_, err := setclient.CoreV1().Secrets(namespace).Update(context.TODO(), cmmodel, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("%+v", err)
		} else {
			fmt.Printf("Secret %s updated\n", secretName)
		}
	}
}
