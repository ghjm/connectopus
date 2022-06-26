import * as React from 'react';
import { Skeleton } from '@patternfly/react-core';
import { usePageVisibility } from 'react-page-visibility';
import { useQuery } from 'urql';
import { useEffect, useRef, useState } from 'react';
import Cytoscape from 'cytoscape';
import CytoscapeComponent from 'react-cytoscapejs';
import nodeImage from '@app/images/node.png';
import equal from 'fast-deep-equal/react';
import cola from 'cytoscape-cola';

Cytoscape.use(cola);

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

interface ICytoscapeComponent {
  data: Array<Record<string, unknown>> | undefined;
  key: number;
}

interface ICytoscapeRendered {
  element: JSX.Element;
  key: number;
}

const cytoscapeComponent = (props: ICytoscapeComponent): ICytoscapeRendered => {
  if (props.data === undefined) {
    return { element: <React.Fragment></React.Fragment>, key: 0 };
  }
  return {
    element: (
      <CytoscapeComponent
        key={props.key}
        elements={props.data}
        userPanningEnabled={false}
        userZoomingEnabled={false}
        boxSelectionEnabled={false}
        autoungrabify={true}
        autounselectify={true}
        // layout={
        //   {
        //     name: 'cose-bilkent',
        //     animate: false,
        //     idealEdgeLength: 50,
        //     weightAttr: "weight",
        //   }
        // }
        layout={{
          name: 'cola',
          animate: false,
          edgeLength: (edge) => {
            return edge.data().edgeLength;
          },
        }}
        autoFit={true}
        style={{ width: '100%', height: '100%' }}
        stylesheet={[
          {
            selector: 'node',
            style: {
              width: 10,
              height: 10,
              'background-fit': 'contain',
              label: 'data(label)',
            },
          },
          {
            selector: 'node[type="node"]',
            style: {
              'background-image': nodeImage,
            },
          },
          {
            selector: 'node[type="resource"]',
            style: {
              width: 5,
              height: 5,
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
              'source-label': 'data(label)',
              'source-text-offset': '15',
              'line-color': 'darkgrey',
              'font-size': 3,
              'target-arrow-shape': 'triangle',
            },
          },
        ]}
      />
    ),
    key: props.key,
  };
};

const Network: React.FunctionComponent = () => {
  const [elementData, setElementData] = useState<Array<Record<string, unknown>> | undefined>(undefined);
  const [myNode, setMyNode] = useState('');
  const [graphKey, setGraphKey] = useState(1);
  const [isPageLoading, setIsPageLoading] = useState(true);
  const [renderedGraph, setRenderedGraph] = useState<ICytoscapeRendered | undefined>(undefined);
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
  useEffect(() => {
    function handleResize() {
      setGraphKey(graphKey + 1);
    }
    window.addEventListener('resize', handleResize);
    return () => {
      window.removeEventListener('resize', handleResize);
    };
  });
  if (!result.fetching && isPageLoading) {
    setIsPageLoading(false);
  }

  const nodes = {};
  const nodeNames = {};
  let curMyNode = '';

  const nodeName = (addr: string) => {
    const name = nodeNames[addr];
    if (name === undefined) {
      return `[${addr}]`;
    }
    return `${name} [${addr}]`;
  };

  if (result.data !== undefined) {
    for (const node in result.data['status']['nodes']) {
      const nodeData = result.data['status']['nodes'][node];
      nodeNames[nodeData['addr']] = nodeData['name'];
      const nodeConns: Record<string, unknown>[] = [];
      for (const conn in nodeData['conns']) {
        const connData = nodeData['conns'][conn];
        nodeConns.push({
          addr: connData['subnet'].replace(/\/128$/, ''),
          cost: connData['cost'],
        });
      }
      nodes[nodeData['addr']] = nodeConns;
    }
    curMyNode = result.data['status']['addr'];
  }

  if (curMyNode != myNode) {
    setMyNode(curMyNode);
    setGraphKey(graphKey + 1);
  }

  const elements: Record<string, unknown>[] = [];
  for (const node in nodes) {
    const selected = node === curMyNode;
    elements.push({ data: { id: node, label: nodeName(node), group: node, type: 'node' }, selected: selected });
  }
  for (const node in nodes) {
    for (const i in nodes[node]) {
      const connNode = nodes[node][i];
      let edgeLength = 50;
      if (nodes[connNode.addr] === undefined) {
        edgeLength = 25;
        elements.push({ data: { id: connNode.addr, label: nodeName(connNode.addr), group: node, type: 'resource' } });
      }
      let edgeLabel = `cost: ${connNode.cost}`;
      if (connNode.cost === 1.0) {
        edgeLabel = '';
      }
      elements.push({
        data: {
          id: `${node}-${connNode.addr}`,
          source: node,
          target: connNode.addr,
          label: edgeLabel,
          edgeLength: edgeLength,
        },
      });
    }
  }

  const elementsChanged = !equal(elements, elementData);
  if (renderedGraph === undefined || graphKey !== renderedGraph.key || elementsChanged) {
    if (elementsChanged) {
      setElementData(elements);
    }
    setRenderedGraph(cytoscapeComponent({ data: elementData, key: graphKey }));
  }

  if (isPageLoading) {
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
  }
  if (result.error) {
    return <p>{JSON.stringify(result.error)}</p>;
  }
  if (renderedGraph === undefined) {
    return <React.Fragment></React.Fragment>;
  }
  return renderedGraph.element;
};

export { Network };
