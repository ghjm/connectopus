import * as React from 'react';
import { PageHeader, Page, Flex, FlexItem, Alert, Button, Grid, GridItem } from '@patternfly/react-core';
import logo from '@app/images/connectopus.png';

const onRefresh = () => {
  window.location.reload();
};

const NoAgent: React.FunctionComponent = () => {
  return (
    <Page header={<PageHeader logo={<img src={logo} alt="Connectopus Logo" />} />}>
      <Grid>
        <GridItem span={1} />
        <GridItem span={6}>
          <Flex direction={{ default: 'column' }} alignSelf={{ default: 'alignSelfCenter' }}>
            <FlexItem style={{ ['paddingTop' as string]: '4rem' }}>
              <Alert
                variant="info"
                isInline
                isPlain
                style={{
                  ['--pf-c-alert__FontSize' as string]: '2rem',
                  ['--pf-c-alert__icon--FontSize' as string]: '2rem',
                }}
                title="No SSH Agent Found"
              />
            </FlexItem>
            <FlexItem style={{ ['paddingTop' as string]: '3rem' }}>
              <p>
                Connectopus uses SSH keys for authentication and configuration bundle signing. To run Connectopus, you
                must have an SSH key. It is best to load this key into an agent, so that Connectopus never has access to
                the private key.
              </p>
              <br />
              <p>
                Graphical desktops generally provide a built-in SSH agent, such as gnome-keyring on Linux, Keychain on
                Mac, and Pageant (part of PuTTY) on Windows.
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
          </Flex>
        </GridItem>
        <GridItem span={5} />
      </Grid>
    </Page>
  );
};

export { NoAgent };
