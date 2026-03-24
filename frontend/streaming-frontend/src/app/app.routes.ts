import { Routes } from '@angular/router';
import { VodPageComponent } from './pages/vod-page/vod-page.component';
import { WatchVodPageComponent } from './pages/watch-vod-page/watch-vod-page.component';

export const appRoutes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app/vod', component: VodPageComponent },
  { path: 'watch/vod/:id', component: WatchVodPageComponent },
  { path: '**', redirectTo: 'app/vod' },
];
