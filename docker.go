package main

import (
    "log"
    "regexp"
    "encoding/base64"
    "strings"

    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/ecr"
    "github.com/google/go-containerregistry/pkg/authn"
    "github.com/google/go-containerregistry/pkg/name"
    "github.com/google/go-containerregistry/pkg/v1/remote"
)

func containerLabel(repoImage string) string {
    imageRef := repoImage

    needsAuth, _ := regexp.MatchString(".*ecr\\..*\\.amazonaws\\.com.*", imageRef)
    auth := authn.Anonymous
    if needsAuth {
        auth = awsAuth(imageRef)
    }

    // Parse the image reference
    ref, err := name.ParseReference(imageRef)
    if err != nil {
        log.Printf("Error parsing image reference: %v\n", err)
        return ""
    }

    // Fetch the remote image descriptor
    img, err := remote.Image(ref, remote.WithAuth(auth))
    if err != nil {
        log.Printf("Error fetching remote image: %v\n", err)
        return ""
    }

    // Get the image config
    configFile, err := img.ConfigFile()
    if err != nil {
        log.Printf("Error getting image config: %v\n", err)
        return ""
    }

    return configFile.Config.Labels["tag_message"]
}

func awsAuth(imageRef string) authn.Authenticator {
    sess, err := session.NewSessionWithOptions(session.Options{ SharedConfigState: session.SharedConfigEnable })
    if err != nil {
        log.Printf("Failed to create AWS session: %v\n", err)
        return authn.Anonymous
    }

    svc := ecr.New(sess)
    authInput := &ecr.GetAuthorizationTokenInput{}
    authOutput, err := svc.GetAuthorizationToken(authInput)
    if err != nil {
        log.Printf("Failed to get ECR authorization token: %v\n", err)
        return authn.Anonymous
    }

    authData := authOutput.AuthorizationData[0]
    token := *authData.AuthorizationToken

    // Decode base64 token (AWS ECR provides it as `user:password`)
    decodedToken, err := base64.StdEncoding.DecodeString(token)
    if err != nil {
        log.Printf("Failed to deconde ECR token: %v\n", err)
        return authn.Anonymous
    }

    parts := strings.SplitN(string(decodedToken), ":", 2)
    username := parts[0]
    password := parts[1]

    // Step 3: Authenticate and fetch image manifest
    auth := authn.FromConfig(authn.AuthConfig{
        Username: username,
        Password: password,
    })
    
    return auth
}
