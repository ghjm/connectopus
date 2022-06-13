import * as React from 'react';
import { Skeleton } from '@patternfly/react-core';
import { usePageVisibility } from 'react-page-visibility';
import { useQuery } from 'urql';
import { useEffect, useRef, useState } from 'react';
import Cytoscape from 'cytoscape';
import CytoscapeComponent from 'react-cytoscapejs';
import nodeImage from '@app/images/node.png';
import equal from 'fast-deep-equal/react';

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import COSEBilkent from 'cytoscape-cose-bilkent';

Cytoscape.use(COSEBilkent);

const statusQuery = `{
  status {
    addr
    nodes {
      name
      addr
      conns {
        subnet
        cost
      }
    }
  }
}`;

const Network: React.FunctionComponent = () => {
  const [elementData, setElementData] = useState<Array<Record<string, unknown>> | undefined>(undefined);
  const [graphKey, setGraphKey] = useState(1);
  const [isPageLoading, setIsPageLoading] = useState(true);
  const pageVisible = useRef(true);
  pageVisible.current = usePageVisibility();
  const [result, reexecuteQuery] = useQuery({
    query: statusQuery,
  });
  useEffect(() => {
    if (result.fetching) return;
    const timerId = setTimeout(() => {
      reexecuteQuery({ requestPolicy: 'network-only' });
    }, 1000);
    return () => clearTimeout(timerId);
  }, [result.fetching, reexecuteQuery]);
  if (!result.fetching && isPageLoading) {
    setIsPageLoading(false);
  }
  if (isPageLoading)
    return (
      <React.Fragment>
        <br />
        <Skeleton />
        <br />
        <Skeleton />
        <br />
        <Skeleton />
      </React.Fragment>
    );
  if (result.error) return <p>{JSON.stringify(result.error)}</p>;

  const nodes = {};
  const nodeNames = {};
  for (const node in result.data['status']['nodes']) {
    const nodeData = result.data['status']['nodes'][node];
    nodeNames[nodeData['addr']] = nodeData['name'];
    const nodeConns: string[] = [];
    for (const conn in nodeData['conns']) {
      const connData = nodeData['conns'][conn];
      nodeConns.push(connData['subnet'].replace(/\/128$/, ''));
    }
    nodes[nodeData['addr']] = nodeConns;
  }

  const nodeName = (addr: string) => {
    const name = nodeNames[addr];
    if (name === undefined) {
      return `[${addr}]`;
    }
    return `${name} [${addr}]`;
  };

  const elements: Record<string, unknown>[] = [];
  for (const node in nodes) {
    const selected = node === result.data['status']['addr'];
    elements.push({ data: { id: node, label: nodeName(node) }, selected: selected });
    for (const conn in nodes[node]) {
      const connNode = nodes[node][conn];
      if (nodes[connNode] === undefined) {
        elements.push({ data: { id: connNode, label: nodeName(connNode) } });
      }
      elements.push({ data: { source: node, target: connNode } });
    }
  }

  if (!equal(elements, elementData)) {
    setElementData(elements);
    setGraphKey(graphKey + 1);
  }

  if (elements.length === 0) {
    return <React.Fragment></React.Fragment>;
  }

  return (
    <CytoscapeComponent
      key={graphKey}
      elements={elements}
      userPanningEnabled={false}
      userZoomingEnabled={false}
      boxSelectionEnabled={false}
      autoungrabify={true}
      autounselectify={true}
      layout={{ name: 'cose-bilkent', animate: false }}
      autoFit={true}
      style={{ width: '100%', height: '100%' }}
      stylesheet={[
        {
          selector: 'node',
          style: {
            width: 10,
            height: 10,
            'background-image': nodeImage,
            'background-fit': 'contain',
            label: 'data(label)',
          },
        },
        {
          selector: 'node[label]',
          style: {
            'text-valign': 'bottom',
            'text-halign': 'center',
            'font-size': 5,
          },
        },
        {
          selector: 'edge',
          style: {
            width: 1,
            'line-color': 'darkgrey',
          },
        },
      ]}
    />
  );
};

export { Network };
