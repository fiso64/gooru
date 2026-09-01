import React from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

window.React = React;

await import('./logos.jsx');
await import('./icons.jsx');
await import('./data.jsx');
await import('./components/SearchBar.jsx');
await import('./components.jsx');
await import('./screens/Login.jsx');
await import('./screens/Library.jsx');
await import('./screens/Lightbox.jsx');
await import('./screens/Others.jsx');
await import('./App.jsx');

createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <window.GooruApp />
  </React.StrictMode>
);
