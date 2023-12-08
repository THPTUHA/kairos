package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/plugin/kairos-executor-docker/runtime"
	"github.com/sirupsen/logrus"
)

type CRE interface {
	// Type returns the container runtime executor type, by default docker
	// is used.
	Type() string

	runtime.RuntimeService

	runtime.ImageService
}

type PipelineSpecWithName struct {
	Name string `json:"name"`

	Workflow string `json:"workflow"`
}

type CRExecutor struct {
	// runtime is the underlying container runtime to use for the
	// ContainerRuntimeExecutor
	runtime string

	// name for the container runtime executor
	// all the resources created by the executor will be related to
	// this name.
	name string

	// cre is the container runtime executor for the Runtime
	// executor
	cre CRE

	// id is the ID corresponding to the current run, it should be unique
	// for each of the CRExecutor instance.
	id string

	// conainerID contains the ID of the conainer running in the current environment
	containerID string

	// log contains the logger for the pipeline executor
	log *logrus.Entry

	// useStore specifies wheather to use kvstore interaction during the execution
	// of the pipeline.
	useStore bool

	mux *sync.Mutex
}

func NewCRExecutor(runtime, id, name string) *CRExecutor {
	return &CRExecutor{
		runtime: runtime,
		name:    name,
		id:      id,
		log: logrus.WithFields(logrus.Fields{
			"pipeline": name,
			"id":       id,
			"runtime":  runtime,
		}),
		useStore: true,
		mux:      &sync.Mutex{},
	}
}

func (e *RuntimeExecutor) Type() string {
	return "docker"
}

func (c *CRExecutor) getResName() string {
	return strings.ToLower(fmt.Sprintf("%s-%s", c.name, c.id))
}

func (c *CRExecutor) Configure() error {
	c.log.Infof("Configuring the container runtime executor")

	cre, err := NewCRE()
	c.cre = cre

	img := parseImageCanonicalURL("node:18-alpine")

	// First set up the image for the container to run.
	// Here we assume that the name is unique for each pipeline run
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	res, err := cre.PullImage(ctx, &runtime.PullImageRequest{
		Image: &runtime.ImageSpec{
			Image: img,
		},
	}, c.getResName())
	if err != nil {
		return fmt.Errorf("error while pulling pipeline image: %s", err)
	}

	// PullImage also tags the image we have just pulled, here remove the original tag
	// from the pulled image.
	err = cre.RemoveImage(context.TODO(), &runtime.RemoveImageRequest{
		Image: &runtime.ImageSpec{
			Tag: res.ImageRef,
		},
	})

	if err != nil && !runtime.IsImageNotFoundError(err) {
		return fmt.Errorf("error removing original tag from the image: %s", err)
	}

	// Also remove the image tag with the original name
	err = cre.RemoveImage(context.TODO(), &runtime.RemoveImageRequest{
		Image: &runtime.ImageSpec{
			Tag: img,
		},
	})

	if err != nil && !runtime.IsImageNotFoundError(err) {
		return fmt.Errorf("error removing original image name from the image: %s", err)
	}

	c.log.Debugf("original image tag name has been removed")

	envs := make([]*runtime.KeyValue, 0)
	// for key, val := range c.spec.Envs {
	// 	envs = append(envs, &runtime.KeyValue{
	// 		Key:   key,
	// 		Value: val,
	// 	})
	// }
	// After pulling in the image for the container
	// create the container with the configuration required.
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*60)
	resp, err := cre.CreateContainer(ctx, &runtime.CreateContainerRequest{
		Config: &runtime.ContainerConfig{
			Metadata: &runtime.ContainerMetadata{
				Name: c.getResName(),
			},
			Image: &runtime.ImageSpec{
				Image: c.getResName(),
			},
			Command:    []string{"sleep", fmt.Sprintf("%d", int64(time.Minute*100/time.Second))},
			WorkingDir: "/",
			Mounts:     []*runtime.Mount{
				// {
				// 	ContainerPath: defaults.AgentMountContainerScript,
				// 	HostPath:      defaults.AgentMountScript,
				// },
			},
			Envs: envs,
		},
	})
	if err != nil {
		return fmt.Errorf("error while creating container: %s", err)
	}

	c.containerID = resp.ContainerID

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()
	err = cre.StartContainer(ctx, &runtime.StartContainerRequest{
		ContainerID: resp.ContainerID,
	})
	if err != nil {
		return fmt.Errorf("error while starting container: %s", err)
	}
	return nil
}

func main() {
	re := NewCRExecutor("docker", "1", "hello")
	err := re.Configure()
	if err != nil {
		fmt.Println(err)
	}
}
