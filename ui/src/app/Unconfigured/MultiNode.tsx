import * as React from 'react';
import { PageHeader, Page, Flex, FlexItem, Grid, GridItem, Alert, Button } from '@patternfly/react-core';
import logo from '@app/images/connectopus.png';

const onRefresh = () => {
  window.location.reload();
};

const MultiNode: React.FunctionComponent = () => {
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
                title="Multiple Connectopus Nodes Found"
              />
            </FlexItem>
            <FlexItem style={{ ['paddingTop' as string]: '3rem' }}>
              <p>Multiple running Connectopus nodes were found.</p>
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

export { MultiNode };
