import * as React from 'react';
import { Route, RouteComponentProps, Switch } from 'react-router-dom';
import { Network } from '@app/Network/Network';
import { GraphQL } from '@app/GraphQL/GraphQL';
import { NotFound } from '@app/NotFound/NotFound';
import { useDocumentTitle } from '@app/utils/useDocumentTitle';
import { Status } from '@app/Status/Status';
import { Config } from '@app/Config/Config';

export interface IAppRoute {
  label?: string; // Excluding the label will exclude the route from the nav sidebar in NoAgent
  /* eslint-disable @typescript-eslint/no-explicit-any */
  component: React.ComponentType<RouteComponentProps<any>> | React.ComponentType<any>;
  /* eslint-enable @typescript-eslint/no-explicit-any */
  exact?: boolean;
  path: string;
  title: string;
  isAsync?: boolean;
  routes?: undefined;
}

export interface IAppRouteGroup {
  label: string;
  routes: IAppRoute[];
}

export type AppRouteConfig = IAppRoute | IAppRouteGroup;

const routes: AppRouteConfig[] = [
  {
    component: Network,
    exact: true,
    label: 'Network',
    path: '/',
    title: 'Connectopus | Network',
  },
  {
    component: Status,
    exact: true,
    isAsync: true,
    label: 'Status',
    path: '/Status',
    title: 'Connectopus | Status',
  },
  {
    component: Config,
    exact: true,
    isAsync: true,
    label: 'Config',
    path: '/Config',
    title: 'Connectopus | Config',
  },
  {
    component: GraphQL,
    exact: true,
    isAsync: true,
    label: 'GraphQL API',
    path: '/GraphQL',
    title: 'Connectopus | GraphQL API',
  },
];

const RouteWithTitleUpdates = ({ component: Component, title, ...rest }: IAppRoute) => {
  useDocumentTitle(title);

  function routeWithTitle(routeProps: RouteComponentProps) {
    return <Component {...rest} {...routeProps} />;
  }

  return <Route render={routeWithTitle} {...rest} />;
};

const PageNotFound = ({ title }: { title: string }) => {
  useDocumentTitle(title);
  return <Route component={NotFound} />;
};

const flattenedRoutes: IAppRoute[] = routes.reduce(
  (flattened, route) => [...flattened, ...(route.routes ? route.routes : [route])],
  [] as IAppRoute[]
);

const AppRoutes = (): React.ReactElement => {
  return (
    <Switch>
      {flattenedRoutes.map(({ path, exact, component, title, isAsync }, idx) => (
        <RouteWithTitleUpdates
          path={path}
          exact={exact}
          component={component}
          key={idx}
          title={title}
          isAsync={isAsync}
        />
      ))}
      <PageNotFound title="404 Page Not Found" />
    </Switch>
  );
};

export { AppRoutes, routes };
