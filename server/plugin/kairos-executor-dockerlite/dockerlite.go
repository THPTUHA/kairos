package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
)

type DockerLite struct {
	cb plugin.StatusHelper
}

func (d *DockerLite) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {
	d.cb = cb
	out, err := d.ExecuteImpl(args)
	resp := &proto.ExecuteResponse{
		Output: out,
	}

	if err != nil {
		resp.Error = err.Error()
	}
	return resp, err
}

func (d *DockerLite) ExecuteImpl(args *proto.ExecuteRequest) ([]byte, error) {
	os.Setenv("DOCKER_API_VERSION", "1.40")
	os.Setenv("DOCKER_DEFAULT_PLATFORM", "linux/amd64")
	action := args.Config["action"]
	imageName := args.Config["imageName"]
	imageTag := args.Config["imageTag"]
	dockerfile := args.Config["dockerfile"]
	contextDir := args.Config["contextDir"]

	if imageTag == "" {
		imageTag = "latest"
	}
	switch action {
	case "build":
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		err := d.buildImage(path.Join(contextDir, dockerfile), contextDir, imageName, imageTag)
		if err != nil {
			return []byte(err.Error()), err
		}
		return []byte(fmt.Sprintf("Build image %s/%s successful", imageName, imageTag)), err
	case "push":
		err := d.pushImage(imageName, imageTag)
		if err != nil {
			return []byte(err.Error()), err
		}
		return []byte(fmt.Sprintf("Push image %s/%s successful", imageName, imageTag)), err
	default:
		return nil, fmt.Errorf("action %s invalid", action)
	}
}

func (d *DockerLite) buildImage(dockerfile, contextDir, imageName, imageTag string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer cli.Close()

	buildContext, err := archive.TarWithOptions(contextDir, &archive.TarOptions{})
	if err != nil {
		return err
	}
	dockerBuildContext, err := os.Open(dockerfile)
	defer dockerBuildContext.Close()

	buildResponse, err := cli.ImageBuild(
		context.Background(),
		buildContext,
		types.ImageBuildOptions{
			Tags:     []string{fmt.Sprintf("%s:%s", imageName, imageTag)},
			Platform: "linux/amd64",
		},
	)
	if err != nil {
		return err
	}
	defer buildResponse.Body.Close()

	if err := d.logBuildResponse(buildResponse.Body); err != nil {
		return err
	}

	return nil
}

func (d *DockerLite) pushImage(imageName, imageTag string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer cli.Close()

	authConfig := types.AuthConfig{
		Username: "nexta2020",
		Password: "Nexta@123",
	}

	authConfigBytes, _ := json.Marshal(authConfig)
	authConfigEncoded := base64.URLEncoding.EncodeToString(authConfigBytes)
	pushResponse, err := cli.ImagePush(
		context.Background(),
		fmt.Sprintf("%s:%s", imageName, imageTag),
		types.ImagePushOptions{
			RegistryAuth: authConfigEncoded,
		},
	)
	if err != nil {
		return err
	}
	defer pushResponse.Close()

	if err := d.logResponse(pushResponse); err != nil {
		return err
	}
	return nil
}

func (d *DockerLite) logResponse(response io.Reader) error {
	scanner := bufio.NewScanner(response)
	for scanner.Scan() {
		d.cb.Update(scanner.Bytes(), true)
	}
	if err := scanner.Err(); err != nil {
		d.cb.Update([]byte(err.Error()), false)
		return err
	}
	return nil
}

func (d *DockerLite) logBuildResponse(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		d.cb.Update(scanner.Bytes(), true)
	}
	if err := scanner.Err(); err != nil {
		d.cb.Update([]byte(err.Error()), false)
		return err
	}
	return nil
}
