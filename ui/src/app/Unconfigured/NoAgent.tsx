import * as React from 'react';
import { FlexItem, Button } from '@patternfly/react-core';
import { Layout } from '@app/Unconfigured/Layout';

const onRefresh = () => {
  window.location.reload();
};

const NoAgent: React.FunctionComponent = () => {
  return (
    <Layout title={'No SSH Agent Found'}>
      <FlexItem style={{ ['paddingTop' as string]: '3rem' }}>
        <p>
          Connectopus uses SSH keys for authentication and configuration bundle signing. To run Connectopus, you must
          have an SSH key. It is best to load this key into an agent, so that Connectopus never has access to the
          private key.
        </p>
        <br />
        <p>
          Graphical desktops generally provide a built-in SSH agent, such as gnome-keyring on Linux, Keychain on Mac,
          and Pageant (part of PuTTY) on Windows.
        </p>
        <br />
        <p>
          If it is not possible to run an agent, you can provide the key directly using command-line options. See{' '}
          <i>connectopus --help</i> for details.
        </p>
        <br />
        <p>Please add at least one SSH key to an agent visible to Connectopus and then click Refresh.</p>
        <br />
        <Button onClick={onRefresh}>Refresh</Button>
      </FlexItem>
    </Layout>
  );
};

export { NoAgent };
