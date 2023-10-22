import * as React from 'react';
import {
  Page,
  Alert,
  TextArea,
  Text,
  Flex,
  FlexItem,
  TextContent,
  TextVariants,
  Button,
  Grid,
  GridItem,
} from '@patternfly/react-core';
import { PageHeader } from '@patternfly/react-core/deprecated';
import logo from '@app/images/connectopus.png';
import { useState } from 'react';
import Cookies from 'universal-cookie';

const Unauthorized: React.FunctionComponent = () => {
  const [value, setValue] = useState('');
  const onSubmit = () => {
    const cookies = new Cookies();
    cookies.set('AuthToken', value, { path: '/' });
    window.location.reload();
  };
  return (
    <Page header={<PageHeader logo={<img src={logo} alt="Connectopus Logo" />} />}>
      <Grid>
        <GridItem span={3}></GridItem>
        <GridItem span={6}>
          <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }}>
            <FlexItem grow={{ default: 'grow' }}></FlexItem>
            <FlexItem grow={{ default: 'grow' }}>
              <Flex direction={{ default: 'column' }} alignSelf={{ default: 'alignSelfCenter' }}>
                <FlexItem style={{ ['paddingTop' as string]: '4rem' }}>
                  <Alert
                    variant="danger"
                    isInline
                    isPlain
                    style={{
                      ['--pf-c-alert__FontSize' as string]: '2rem',
                      ['--pf-c-alert__icon--FontSize' as string]: '2rem',
                    }}
                    title="Unauthorized"
                  />
                </FlexItem>
                <FlexItem style={{ ['paddingTop' as string]: '3rem' }}>
                  <TextContent>
                    <Text component={TextVariants.h2}>Please enter an authorization token.</Text>
                  </TextContent>
                </FlexItem>
                <FlexItem style={{ ['paddingTop' as string]: '0.5rem' }}>
                  <TextContent>
                    <Text component={TextVariants.h4}>
                      You can generate an authorization token using <i>connectopus get-token</i> from the command line.
                    </Text>
                  </TextContent>
                </FlexItem>
                <FlexItem style={{ ['paddingTop' as string]: '0.5rem' }}>
                  <TextArea
                    value={value}
                    type="text"
                    onChange={(_event, val) => setValue(val)}
                    aria-label="token input field"
                  />
                </FlexItem>
                <FlexItem
                  style={{ ['paddingTop' as string]: '0.5rem', ['paddingBottom' as string]: '2rem' }}
                  alignSelf={{ default: 'alignSelfFlexEnd' }}
                >
                  <Button onClick={onSubmit}>Submit</Button>
                </FlexItem>
              </Flex>
            </FlexItem>
            <FlexItem grow={{ default: 'grow' }}></FlexItem>
          </Flex>
        </GridItem>
      </Grid>
    </Page>
  );
};

export { Unauthorized };
