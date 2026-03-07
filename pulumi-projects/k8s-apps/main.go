package main

import (
	"fmt"

	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		kubectx := config.New(ctx, "kubernetes").Require("context")

		_, err := yamlv2.NewConfigGroup(ctx, "manifests",
			&yamlv2.ConfigGroupArgs{
				Files: pulumi.ToStringArray([]string{fmt.Sprintf("./manifests/%s/*.yaml", kubectx)}),
			},
		)
		if err != nil {
			return err
		}
		return nil
	})
}
