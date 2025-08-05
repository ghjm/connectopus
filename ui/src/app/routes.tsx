import * as React from 'react';
import { Route, Routes } from 'react-router-dom';
import { Network } from '@app/Network/Network';
import { GraphQL } from '@app/GraphQL/GraphQL';
import { NotFound } from '@app/NotFound/NotFound';
import { useDocumentTitle } from '@app/utils/useDocumentTitle';
import { Status } from '@app/Status/Status';
import { Config } from '@app/Config/Config';

export interface IAppRoute {
  label?: string; // Excluding the label will exclude the route from the nav sidebar in NoAgent
  /* eslint-disable @typescript-eslint/no-explicit-any */
  component: React.ComponentType<any>;
  /* eslint-enable @typescript-eslint/no-explicit-any */
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
    label: 'Network',
    path: '/',
    title: 'Connectopus | Network',
  },
  {
    component: Status,
    isAsync: true,
    label: 'Status',
    path: '/Status',
    title: 'Connectopus | Status',
  },
  {
    component: Config,
    isAsync: true,
    label: 'Config',
    path: '/Config',
    title: 'Connectopus | Config',
  },
  {
    component: GraphQL,
    isAsync: true,
    label: 'GraphQL API',
    path: '/GraphQL',
    title: 'Connectopus | GraphQL API',
  },
];

const RouteWithTitleUpdates = ({ component: Component, title, ...rest }: IAppRoute) => {
  useDocumentTitle(title);
  return <Component {...rest} />;
};

const PageNotFound = ({ title }: { title: string }) => {
  useDocumentTitle(title);
  return <NotFound />;
};

const flattenedRoutes: IAppRoute[] = routes.reduce(
  (flattened, route) => [...flattened, ...(route.routes ? route.routes : [route])],
  [] as IAppRoute[],
);

const AppRoutes = (): React.ReactElement => {
  return (
    <Routes>
      {flattenedRoutes.map(({ path, component, title, isAsync }, idx) => (
        <Route
          key={idx}
          path={path}
          element={<RouteWithTitleUpdates component={component} path={path} title={title} isAsync={isAsync} />}
        />
      ))}
      <Route path="*" element={<PageNotFound title="404 Page Not Found" />} />
    </Routes>
  );
};

export { AppRoutes, routes };
