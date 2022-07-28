import * as React from 'react';
import { FlexItem, Skeleton, Menu, MenuContent, MenuList, MenuItem } from '@patternfly/react-core';
import { gql, useMutation, useQuery } from 'urql';
import { useState } from 'react';
import { Layout } from '@app/Unconfigured/Layout';

const nodesQuery = gql`
  {
    availableNodes {
      node
    }
  }
`;

const selectNodeMutation = gql`
  mutation SelectNodeMutation($node: String!) {
    selectNode(node: $node) {
      mutationID
    }
  }
`;

const MultiNode: React.FunctionComponent = () => {
  const [result] = useQuery({ query: nodesQuery });
  const [, executeMutation] = useMutation(selectNodeMutation);
  const [activeItem, setActiveItem] = useState('');
  const [isPageLoading, setIsPageLoading] = useState(true);
  if (!result.fetching && isPageLoading) {
    setIsPageLoading(false);
  }
  const onClick = (itemID) => {
    setActiveItem(itemID);
    executeMutation({ node: itemID }).then(() => {
      window.location.reload();
    });
  };
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
  return (
    <Layout title={'Multiple Nodes Found'}>
      <FlexItem style={{ ['paddingTop' as string]: '1rem' }}>
        <p>Please select the node you would like to connect to:</p>
        <FlexItem style={{ ['paddingTop' as string]: '2rem', ['paddingBottom' as string]: '2rem' }}>
          <Menu activeItemId={activeItem}>
            <MenuContent>
              <MenuList>
                {result.data['availableNodes']['node'].map((node) => (
                  <MenuItem
                    key={node}
                    itemID={node}
                    onClick={() => {
                      onClick(node);
                    }}
                  >
                    {node}
                  </MenuItem>
                ))}
              </MenuList>
            </MenuContent>
          </Menu>
        </FlexItem>
      </FlexItem>
    </Layout>
  );
};

export { MultiNode };
