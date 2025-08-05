const { merge } = require('webpack-merge');
const common = require('./webpack.common.js');
const HOST = process.env.HOST || 'localhost';
const PORT = process.env.PORT || '9000';

module.exports = merge(common('development'), {
  mode: 'development',
  devtool: 'eval-source-map',
  devServer: {
    host: HOST,
    port: PORT,
    compress: true,
    historyApiFallback: true,
    open: true,
    proxy: [
      {
        context: ['/query'],
        target: 'http://localhost:8123',
      },
      {
        context: ['/localquery'],
        target: 'http://localhost:26663',
      },
      {
        context: ['/proxy'],
        target: 'http://localhost:8123',
      },
      {
        context: ['/api'],
        target: 'http://localhost:8123',
      },
    ],
  },
  module: {
    rules: [
      {
        test: /\.css$/,
        //include: [...stylePaths],
        use: ['style-loader', 'css-loader'],
      },
    ],
  },
});
