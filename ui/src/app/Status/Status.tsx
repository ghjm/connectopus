import * as React from 'react';
import {
  Skeleton,
  Card,
  CardTitle,
  CardBody,
  DescriptionList,
  DescriptionListTerm,
  DescriptionListDescription,
  DescriptionListGroup,
  PageSection,
  Stack,
  StackItem,
} from '@patternfly/react-core';
import { Table /* data-codemods */, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { usePageVisibility } from 'react-page-visibility';
import { useQuery } from 'urql';
import { useEffect, useRef, useState } from 'react';

const statusQuery = `{
  status {
    name
    addr
    sessions {
      addr
      connected
      conn_start
    }
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

const Status: React.FunctionComponent = () => {
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
      <PageSection>
        <br />
        <Skeleton />
        <br />
        <Skeleton />
        <br />
        <Skeleton />
      </PageSection>
    );
  if (result.error)
    return (
      <PageSection>
        <p>{JSON.stringify(result.error)}</p>
      </PageSection>
    );
  return (
    <PageSection>
      <Stack hasGutter>
        <StackItem>
          <Card>
            <CardTitle>Node</CardTitle>
            <CardBody>
              <DescriptionList columnModifier={{ default: '2Col' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Name</DescriptionListTerm>
                  <DescriptionListDescription>{result.data['status']['name']}</DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Address</DescriptionListTerm>
                  <DescriptionListDescription>{result.data['status']['addr']}</DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </StackItem>
        <StackItem>
          <Card>
            <CardTitle>Connected Sessions</CardTitle>
            <CardBody>
              <Table variant={'compact'}>
                <Thead>
                  <Tr>
                    <Th>Address</Th>
                    <Th>Connected Since</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {result.data['status']['sessions'].map((sess) => (
                    <Tr key={sess['addr']}>
                      <Td>{sess['addr']}</Td>
                      <Td>{sess['connected'] ? sess['conn_start'] : 'not connected'}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        </StackItem>
        <StackItem>
          <Card>
            <CardTitle>Known Nodes</CardTitle>
            <CardBody>
              <Table variant={'compact'}>
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    <Th>Address</Th>
                    <Th>Connections</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {result.data['status']['nodes'].map((sess) => (
                    <Tr key={sess['node']}>
                      <Td>{sess['name']}</Td>
                      <Td>{sess['addr']}</Td>
                      <Td>
                        <Table variant={'compact'}>
                          <Thead>
                            <Tr>
                              <Th>Subnet</Th>
                              <Th>Cost</Th>
                            </Tr>
                          </Thead>
                          <Tbody>
                            {sess['conns'].map((conn) => (
                              <Tr key={conn['subnet']}>
                                <Td>{conn['subnet']}</Td>
                                <Td>{conn['cost']}</Td>
                              </Tr>
                            ))}
                          </Tbody>
                        </Table>
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        </StackItem>
      </Stack>
    </PageSection>
  );
};

export { Status };
