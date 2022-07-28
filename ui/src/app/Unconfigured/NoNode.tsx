import * as React from 'react';
import { FlexItem, Button } from '@patternfly/react-core';
import { Layout } from '@app/Unconfigured/Layout';

const onRefresh = () => {
  window.location.reload();
};

const NoNode: React.FunctionComponent = () => {
  return (
    <Layout title={'No Node Found'}>
      <FlexItem style={{ ['paddingTop' as string]: '3rem' }}>
        <p>No running Connectopus node was found.</p>
        <br />
        <Button onClick={onRefresh}>Refresh</Button>
      </FlexItem>
    </Layout>
  );
};

export { NoNode };
