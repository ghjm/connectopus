import * as React from 'react';
import {
  Page,
  Alert,
  TextArea,
  Content,
  Flex,
  FlexItem,
  Button,
  Grid,
  GridItem,
  Masthead,
  MastheadBrand,
  MastheadMain,
  Brand,
} from '@patternfly/react-core';
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
    <Page
      masthead={
        <Masthead
          style={
            {
              '--pf-v6-c-masthead--BackgroundColor': 'var(--pf-t--color--black)',
            } as React.CSSProperties
          }
        >
          <MastheadMain>
            <MastheadBrand>
              <Brand src={logo} alt="Connectopus Logo" />
            </MastheadBrand>
          </MastheadMain>
        </Masthead>
      }
    >
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
                  <Content>
                    <Content component="h2">Please enter an authorization token.</Content>
                  </Content>
                </FlexItem>
                <FlexItem style={{ ['paddingTop' as string]: '0.5rem' }}>
                  <Content>
                    <Content component="h4">
                      You can generate an authorization token using <i>connectopus get-token</i> from the command line.
                    </Content>
                  </Content>
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
