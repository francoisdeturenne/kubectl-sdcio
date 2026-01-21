package client

import (
	"context"
	"strings"

	configCR "github.com/sdcio/config-server/pkg/generated/clientset/versioned"
	"github.com/sdcio/kubectl-sdcio/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type ConfigClient struct {
	c *configCR.Clientset
}

// filtering values
type BlameFilter struct {
	LeafName  string
	Owner     string
	Path      string
	Deviation bool
}

func NewConfigClient(restConfig *rest.Config) (*ConfigClient, error) {
	clientset, err := configCR.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return &ConfigClient{
		c: clientset,
	}, nil
}

func (c *ConfigClient) GetBlameTree(ctx context.Context, namespace string, device string) (*sdcpb.BlameTreeElement, error) {
	resp, err := c.c.ConfigV1alpha1().ConfigBlames(namespace).Get(ctx, device, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	bte := &sdcpb.BlameTreeElement{}
	err = protojson.Unmarshal([]byte(resp.Status.Value.Raw), bte)
	if err != nil {
		return nil, err
	}
	return bte, nil
}

func (c *ConfigClient) GetFilteredBlameTree(ctx context.Context, namespace string, device string, filter BlameFilter) (*sdcpb.BlameTreeElement, error) {
	tree, err := c.GetBlameTree(ctx, namespace, device)
	if err != nil {
		return nil, err
	}

	// if no filter criteria, returns whole tree
	if filter.LeafName == "" && filter.Owner == "" && filter.Path == "" && !filter.Deviation {
		return tree, nil
	}
	return c.filterBlameTree(tree, []string{}, filter), nil
}

// filterBlameTree filter blame tree keeping the whole path
func (c *ConfigClient) filterBlameTree(node *sdcpb.BlameTreeElement, currentPath []string, filter BlameFilter) *sdcpb.BlameTreeElement {
	if node == nil {
		return nil
	}

	// new node for filtered item
	filteredNode := &sdcpb.BlameTreeElement{
		Name:           node.Name,
		Owner:          node.Owner,
		Value:          node.Value,
		DeviationValue: node.DeviationValue,
	}

	// if node is a child, check matching filter
	if len(node.Childs) == 0 {
		if c.matchesFilter(node, currentPath, filter) {
			return filteredNode
		}
		return nil
	}

	// if node has childs, recursive call on each one
	var filteredChilds []*sdcpb.BlameTreeElement
	newPath := append(currentPath, node.Name)

	for _, child := range node.Childs {
		filteredChild := c.filterBlameTree(child, newPath, filter)
		if filteredChild != nil {
			filteredChilds = append(filteredChilds, filteredChild)
		}
	}

	// if one child matches, keep the node
	if len(filteredChilds) > 0 {
		filteredNode.Childs = filteredChilds
		return filteredNode
	}

	return nil
}

// matchesFilter checks one node against filtering criteria
func (c *ConfigClient) matchesFilter(node *sdcpb.BlameTreeElement, path []string, filter BlameFilter) bool {
	leafMatches := true
	ownerMatches := true
	pathMatches := true
	deviationMatches := true

	// check leaf name filter
	if filter.LeafName != "" {
		leafMatches = utils.MatchesPattern(node.Name, filter.LeafName, false)
	}

	// check owner filter
	if filter.Owner != "" {
		ownerMatches = utils.MatchesPattern(node.Owner, filter.Owner, false)
	}

	// check whole path filter
	if filter.Path != "" {
		fullPath := strings.Join(append(path, node.Name), "/")
		pathMatches = utils.MatchesPattern(fullPath, filter.Path, false)
	}
	// check deviation filter
	if filter.Deviation {
		deviationMatches = node.IsDeviated()
	}

	return leafMatches && ownerMatches && pathMatches && deviationMatches
}

func (c *ConfigClient) GetTargetNames(ctx context.Context, namespace string) ([]string, error) {
	resp, err := c.c.InvV1alpha1().Targets(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(resp.Items))

	for _, i := range resp.Items {
		result = append(result, i.Name)
	}

	return result, nil
}
