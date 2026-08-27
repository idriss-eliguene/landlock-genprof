package main

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	controllerTag = "ghcr.io/flavio/podlock/controller:v0.1.0"
	controllerPin = "ghcr.io/flavio/podlock/controller@sha256:c056fa01ced83ff55bc34037c5a9f6fcf2119d8125b54e8bfe3f1f46f6ebf8e7"
	nriTag        = "ghcr.io/flavio/podlock/nri:v0.1.0"
	nriPin        = "ghcr.io/flavio/podlock/nri@sha256:b22544b9e308b68b47f9d5a01999f23555a56a7547e660bbaa4e1bf6e7bbbc31"
)

type target struct {
	kind, name, section, container, tag, pin string
}

var targets = []target{
	{kind: "DaemonSet", name: "podlock-nri-plugin", section: "initContainers", container: "detect-landlock-support", tag: nriTag, pin: nriPin},
	{kind: "DaemonSet", name: "podlock-nri-plugin", section: "containers", container: "nri", tag: nriTag, pin: nriPin},
	{kind: "Deployment", name: "podlock-controller", section: "containers", container: "controller", tag: controllerTag, pin: controllerPin},
}

func scalar(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapping(m *yaml.Node, key string) *yaml.Node {
	n := scalar(m, key)
	if n != nil && n.Kind == yaml.MappingNode {
		return n
	}
	return nil
}

func sequence(m *yaml.Node, key string) *yaml.Node {
	n := scalar(m, key)
	if n != nil && n.Kind == yaml.SequenceNode {
		return n
	}
	return nil
}

func findWorkload(root *yaml.Node, t target) ([]*yaml.Node, error) {
	if root.Kind == yaml.DocumentNode {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}
	if scalar(root, "kind") == nil || scalar(root, "kind").Value != t.kind || scalar(mapping(root, "metadata"), "name") == nil || scalar(mapping(root, "metadata"), "name").Value != t.name {
		return nil, nil
	}
	spec := mapping(mapping(root, "spec"), "template")
	spec = mapping(spec, "spec")
	items := sequence(spec, t.section)
	if items == nil {
		return nil, fmt.Errorf("%s/%s missing %s", t.kind, t.name, t.section)
	}
	var matches []*yaml.Node
	for _, item := range items.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		name := scalar(item, "name")
		image := scalar(item, "image")
		if name != nil && name.Value == t.container {
			if image == nil {
				return nil, fmt.Errorf("%s/%s container %s has no image", t.kind, t.name, t.container)
			}
			matches = append(matches, image)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("%s/%s expected one %s container %s, found %d", t.kind, t.name, t.section, t.container, len(matches))
	}
	return matches, nil
}

func podSpec(root *yaml.Node) *yaml.Node {
	kind := scalar(root, "kind")
	if kind == nil {
		return nil
	}
	if kind.Value == "Pod" {
		return mapping(root, "spec")
	}
	if kind.Value == "CronJob" {
		return mapping(mapping(mapping(mapping(root, "spec"), "jobTemplate"), "spec"), "template")
	}
	return mapping(mapping(mapping(root, "spec"), "template"), "spec")
}

func podImages(root *yaml.Node, out *[]*yaml.Node) {
	spec := podSpec(root)
	if spec == nil {
		return
	}
	for _, section := range []string{"initContainers", "containers"} {
		items := sequence(spec, section)
		if items == nil {
			continue
		}
		for _, item := range items.Content {
			if image := scalar(item, "image"); image != nil && image.Kind == yaml.ScalarNode {
				*out = append(*out, image)
			}
		}
	}
}

func transform(docs []*yaml.Node) error {
	allowedPins := make(map[*yaml.Node]bool)
	for _, d := range docs {
		for _, t := range targets {
			matches, err := findWorkload(d, t)
			if err != nil {
				return err
			}
			if len(matches) == 1 {
				if matches[0].Value != t.tag {
					return fmt.Errorf("%s/%s %s %s image is %q, want %q", t.kind, t.name, t.section, t.container, matches[0].Value, t.tag)
				}
				matches[0].Value = t.pin
				allowedPins[matches[0]] = true
			}
		}
	}
	for _, t := range targets {
		count := 0
		for _, d := range docs {
			matches, _ := findWorkload(d, t)
			count += len(matches)
		}
		if count != 1 {
			return fmt.Errorf("expected %s/%s %s %s exactly once, found %d", t.kind, t.name, t.section, t.container, count)
		}
	}
	var images []*yaml.Node
	for _, d := range docs {
		if d.Kind == yaml.DocumentNode && len(d.Content) > 0 {
			podImages(d.Content[0], &images)
		}
	}
	for _, image := range images {
		if image.Value == controllerTag || image.Value == nriTag {
			return fmt.Errorf("mutable PodLock image remains: %s", image.Value)
		}
		if (image.Value == controllerPin || image.Value == nriPin) && !allowedPins[image] {
			return fmt.Errorf("pinned PodLock image appears at an unexpected location: %s", image.Value)
		}
	}
	return nil
}

func run(in io.Reader, out io.Writer) error {
	var docs []*yaml.Node
	dec := yaml.NewDecoder(in)
	for {
		var d yaml.Node
		err := dec.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("decode YAML: %w", err)
		}
		if len(d.Content) != 0 {
			docs = append(docs, &d)
		}
	}
	if err := transform(docs); err != nil {
		return err
	}
	enc := yaml.NewEncoder(out)
	defer enc.Close()
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			return fmt.Errorf("encode YAML: %w", err)
		}
	}
	return nil
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
