import * as React from 'react';
import {
  Page,
  Flex,
  FlexItem,
  Grid,
  GridItem,
  Alert,
  Masthead,
  MastheadBrand,
  MastheadMain,
  Brand,
} from '@patternfly/react-core';
import logo from '@app/images/connectopus.png';

const Layout: React.FunctionComponent<{ title: string; children: React.ReactNode }> = (props) => (
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
      <GridItem span={1} />
      <GridItem span={6}>
        <Flex direction={{ default: 'column' }} alignSelf={{ default: 'alignSelfCenter' }}>
          <FlexItem style={{ ['paddingTop' as string]: '2rem' }}>
            <Alert
              variant="info"
              isInline
              isPlain
              style={{
                ['--pf-c-alert__FontSize' as string]: '2rem',
                ['--pf-c-alert__icon--FontSize' as string]: '2rem',
              }}
              title={props.title}
            />
          </FlexItem>
          <FlexItem style={{ ['paddingTop' as string]: '1rem' }}>{props.children}</FlexItem>
        </Flex>
      </GridItem>
      <GridItem span={5} />
    </Grid>
  </Page>
);

export { Layout };
