import * as React from 'react';
import '@patternfly/react-core/dist/styles/base.css';
import { BrowserRouter as Router } from 'react-router-dom';
import { AppLayout } from '@app/AppLayout/AppLayout';
import { AppRoutes } from '@app/routes';
import '@app/app.css';
import { createClient, Provider } from 'urql';

const client = createClient({
  url: '/query',
});

const App: React.FunctionComponent = () => (
  <Router>
    <AppLayout>
      <Provider value={client}>
        <AppRoutes />
      </Provider>
    </AppLayout>
  </Router>
);

export default App;
