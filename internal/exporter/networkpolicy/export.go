// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package networkpolicy converts a Behavior IR (internal/profile) into a
// Kubernetes NetworkPolicy and serializes it to YAML.
//
// This is a sibling of internal/exporter/podlock, not a variant of it:
// PodLock's own CRD schema has no field for network rights at all (see
// internal/exporter/podlock), so network data — held back from tracing
// for exactly that reason (see docs/roadmap.md) — needed its own
// destination. internal/profile and internal/policy know nothing about
// either exporter: the dependency runs exporter -> IR, never the other
// way (see docs/architecture.md).
package networkpolicy

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	yamlv3 "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

// PolicyMeta identifies the pod a BehaviorProfile applies to, and how the
// generated NetworkPolicy should select it.
type PolicyMeta struct {
	Name      string // name of the generated NetworkPolicy
	Namespace string
	// PodLabels are the traced pod's own labels (see k8s.TargetPod.Labels),
	// copied into spec.podSelector.matchLabels. A NetworkPolicy targets
	// workloads by label, not by one pod's identity — this also means the
	// generated policy naturally applies to the whole Deployment/
	// ReplicaSet the traced pod belongs to, not just the one replica that
	// happened to be traced, matching how NetworkPolicy is meant to be
	// used.
	PodLabels map[string]string
}

// ToPolicy converts a BehaviorProfile's network observations into a
// NetworkPolicy ready to be serialized.
//
// Each NetworkAccess becomes one port entry in an Ingress or Egress rule
// depending on its Direction, with no From/To peer restriction: the
// tracer knows a destination/source *port* was involved, not a peer pod
// or service identity, so restricting From/To would be fabricating data
// that wasn't observed — the same "only encode what was actually seen"
// policy internal/exporter/podlock.categoryFor follows.
//
// spec.policyTypes only includes Ingress/Egress for the directions
// actually present: an empty Ingress policy type with no rules would mean
// "deny all ingress" per the NetworkPolicy spec, which is not something
// this tool should assert from the mere absence of observed data.
func ToPolicy(meta PolicyMeta, net profile.NetworkProfile) *networkingv1.NetworkPolicy {
	var ingressPorts, egressPorts []networkingv1.NetworkPolicyPort

	for _, access := range net.Accesses {
		port := networkingv1.NetworkPolicyPort{
			Protocol: protocolTCP(),
			Port:     intOrStringPort(access.Port),
		}
		switch access.Direction {
		case profile.DirectionIngress:
			ingressPorts = append(ingressPorts, port)
		case profile.DirectionEgress:
			egressPorts = append(egressPorts, port)
		}
	}

	sortPorts(ingressPorts)
	sortPorts(egressPorts)

	spec := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: meta.PodLabels},
	}
	if len(ingressPorts) > 0 {
		spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: ingressPorts}}
		spec.PolicyTypes = append(spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	}
	if len(egressPorts) > 0 {
		spec.Egress = []networkingv1.NetworkPolicyEgressRule{{Ports: egressPorts}}
		spec.PolicyTypes = append(spec.PolicyTypes, networkingv1.PolicyTypeEgress)
	}

	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "NetworkPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Spec: spec,
	}
}

// protocolTCP returns a pointer to corev1.ProtocolTCP: trace_tcpconnect/
// trace_bind (and Landlock's own network rights) only cover TCP today —
// see profile.NetworkAccess's doc comment.
func protocolTCP() *corev1.Protocol {
	p := corev1.ProtocolTCP
	return &p
}

func intOrStringPort(port int) *intstr.IntOrString {
	v := intstr.FromInt32(int32(port))
	return &v
}

// sortPorts orders ports ascending for deterministic YAML output, matching
// internal/policy.Synthesize's own sorted output.
func sortPorts(ports []networkingv1.NetworkPolicyPort) {
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port.IntValue() < ports[j].Port.IntValue()
	})
}

// ToYAML serializes a NetworkPolicy to YAML, as written to
// networkpolicy.yaml by the CLI (see cmd/landlock-genprof), with a
// trailing `# confidence: ...` comment on each port — net is the same
// value ToPolicy converted, carrying the per-access Confidence that
// conversion doesn't otherwise preserve (the real NetworkPolicy schema
// has no field for it). Comments are invisible to kubectl apply and to
// this package's own round-trip test — purely for the human doing the
// mandatory review (docs/threat-model.md). See
// internal/exporter/podlock.ToYAML for the same mechanism and why
// re-parsing sigs.k8s.io/yaml's own output (rather than encoding p
// directly with yaml.v3) is what keeps the exact key casing already
// tested.
func ToYAML(p *networkingv1.NetworkPolicy, net profile.NetworkProfile) ([]byte, error) {
	raw, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("re-parsing generated YAML to attach confidence comments: %w", err)
	}
	annotateConfidence(&doc, "", confidenceByPort(net))

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("re-encoding YAML with confidence comments: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// netPortKey identifies one (port, direction) pair — mirrors
// internal/policy/synthesize.go's own private netKey, not shared code:
// this package doesn't import internal/policy.
type netPortKey struct {
	port      int
	direction profile.NetworkDirection
}

// confidenceByPort indexes net by (port, direction) for
// annotateConfidence's lookups.
func confidenceByPort(net profile.NetworkProfile) map[netPortKey]profile.Confidence {
	m := make(map[netPortKey]profile.Confidence, len(net.Accesses))
	for _, a := range net.Accesses {
		m[netPortKey{a.Port, a.Direction}] = a.Confidence
	}
	return m
}

// annotateConfidence walks a parsed NetworkPolicy YAML tree and sets a
// LineComment on each port scalar found under spec.ingress/spec.egress,
// if confidenceByPort has a non-empty Confidence for its (port,
// direction) — direction is tracked through the recursion as "ingress"/
// "egress" keys are encountered, since the same port number can appear
// under both with independently correct confidence. A Confidence of ""
// (the zero value) is left uncommented, same reasoning as
// internal/exporter/podlock.annotateConfidence.
func annotateConfidence(node *yamlv3.Node, direction profile.NetworkDirection, confidenceByPort map[netPortKey]profile.Confidence) {
	switch node.Kind {
	case yamlv3.DocumentNode, yamlv3.SequenceNode:
		for _, child := range node.Content {
			annotateConfidence(child, direction, confidenceByPort)
		}
	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			switch key.Value {
			case "ingress":
				annotateConfidence(value, profile.DirectionIngress, confidenceByPort)
			case "egress":
				annotateConfidence(value, profile.DirectionEgress, confidenceByPort)
			case "port":
				if port, err := strconv.Atoi(value.Value); err == nil {
					if c, ok := confidenceByPort[netPortKey{port, direction}]; ok && c != "" {
						value.LineComment = "confidence: " + string(c)
					}
				}
			default:
				annotateConfidence(value, direction, confidenceByPort)
			}
		}
	}
}
