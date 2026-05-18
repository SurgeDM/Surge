// SolidJS entry point
import { render } from 'solid-js/web';
import App from './App';

const container = document.getElementById('app');
if (container) {
  render(() => <App />, container);
}
