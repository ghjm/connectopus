import * as React from 'react';
import { Skeleton } from '@patternfly/react-core';
import { usePageVisibility } from 'react-page-visibility';
import { useQuery } from 'urql';
import { useEffect, useRef, useState } from 'react';
import Cytoscape from 'cytoscape';
import CytoscapeComponent from 'react-cytoscapejs';
import nodeImage from '@app/images/node.png';

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import COSEBilkent from 'cytoscape-cose-bilkent';

Cytoscape.use(COSEBilkent);

const statusQuery = `{
  status {
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
  const cyRef = useRef<Cytoscape.Core>();
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

  const elements: object[] = [];
  for (const node in nodes) {
    elements.push({ data: { id: node, label: nodeName(node) } });
    for (const conn in nodes[node]) {
      const connNode = nodes[node][conn];
      if (nodes[connNode] === undefined) {
        elements.push({ data: { id: connNode, label: nodeName(connNode) } });
      }
      elements.push({ data: { source: node, target: connNode } });
    }
  }

  if (elements.length === 0) {
    return <React.Fragment></React.Fragment>;
  }

  if (cyRef.current != undefined) {
    try {
      cyRef.current.fit();
    } catch (error) {
      console.log(`caught Cytoscape error: ${error}`);
    }
  }

  return (
    <CytoscapeComponent
      elements={elements}
      cy={(cy): void => {
        cyRef.current = cy;
      }}
      userPanningEnabled={false}
      userZoomingEnabled={false}
      boxSelectionEnabled={false}
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
