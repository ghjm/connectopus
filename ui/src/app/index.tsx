import * as React from 'react';
import '@patternfly/react-core/dist/styles/base.css';
import { BrowserRouter as Router } from 'react-router-dom';
import { AppLayout } from '@app/AppLayout/AppLayout';
import { AppRoutes } from '@app/routes';
import '@app/app.css';
import { Unauthorized } from '@app/Unauthorized/Unauthorized';
import { NoAgent } from '@app/Unconfigured/NoAgent';
import { NoNode } from '@app/Unconfigured/NoNode';
import { MultiNode } from '@app/Unconfigured/MultiNode';
import { Client, createClient, Provider, cacheExchange, fetchExchange } from 'urql';

const App: React.FunctionComponent = () => {
  const [client] = React.useState<Client>(
    createClient({ url: '/localquery', exchanges: [cacheExchange, fetchExchange] })
  );
  const serverData = window['__SERVER_DATA__'];
  if (serverData !== undefined) {
    const pageSel = serverData['page_select'];
    if (pageSel === 'unauthorized') {
      return <Unauthorized />;
    }
    if (pageSel === 'no-agent') {
      return (
        <Provider value={client}>
          <NoAgent />
        </Provider>
      );
    }
    if (pageSel === 'no-node') {
      return (
        <Provider value={client}>
          <NoNode />
        </Provider>
      );
    }
    if (pageSel === 'multi-node') {
      return (
        <Provider value={client}>
          <MultiNode />
        </Provider>
      );
    }
  }
  return (
    <Router>
      <AppLayout>
        <AppRoutes />
      </AppLayout>
    </Router>
  );
};

export default App;
