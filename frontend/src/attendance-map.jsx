import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { useEffect, useRef, useState } from 'react';

// Peta lokasi absensi: menandai posisi masuk (biru) dan pulang (ungu),
// serta area kantor (lingkaran). Titik di luar area ditandai merah.
export function AttendanceMap({ markers, office }) {
  const containerRef = useRef(null);
  const mapRef = useRef(null);
  const layerRef = useRef(null);
  const [error, setError] = useState('');

  useEffect(() => {
    try {
      if (!mapRef.current && containerRef.current) {
        mapRef.current = L.map(containerRef.current, { zoomControl: true, attributionControl: true });
        L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
          maxZoom: 19,
          attribution: '&copy; OpenStreetMap',
        }).addTo(mapRef.current);
        layerRef.current = L.layerGroup().addTo(mapRef.current);
      }
      const layer = layerRef.current;
      if (!layer) return;
      layer.clearLayers();
      const bounds = [];
      if (office?.latitude != null && office?.longitude != null) {
        L.circle([office.latitude, office.longitude], {
          radius: office.radius_meters || 50,
          color: '#25857e', weight: 2, fillColor: '#25857e', fillOpacity: 0.12,
        }).bindPopup(`<strong>Kantor ${office.name || ''}</strong><br/>Radius ${office.radius_meters || 50} m`).addTo(layer);
        bounds.push([office.latitude, office.longitude]);
      }
      markers.filter((item) => item.latitude != null && item.longitude != null).forEach((item) => {
        const color = item.outside ? '#c0452c' : '#1f6fd1';
        const icon = L.divIcon({
          className: 'attendance-pin',
          html: `<span style="background:${color}"></span>`,
          iconSize: [18, 18], iconAnchor: [9, 9],
        });
        L.marker([item.latitude, item.longitude], { icon })
          .bindPopup(`<strong>${item.title}</strong><br/>${item.detail || ''}`)
          .addTo(layer);
        bounds.push([item.latitude, item.longitude]);
      });
      if (bounds.length === 0) {
        mapRef.current.setView([-6.2, 106.8167], 11);
      } else if (bounds.length === 1) {
        mapRef.current.setView(bounds[0], 16);
      } else {
        mapRef.current.fitBounds(bounds, { padding: [24, 24], maxZoom: 17 });
      }
      setTimeout(() => mapRef.current?.invalidateSize(), 150);
    } catch (mapError) {
      setError(mapError.message);
    }
  }, [markers, office]);

  useEffect(() => () => {
    mapRef.current?.remove();
    mapRef.current = null;
    layerRef.current = null;
  }, []);

  if (error) return <div className="alert error" role="alert">{error}</div>;
  return <div className="attendance-map" ref={containerRef} role="img" aria-label="Peta lokasi absensi" />;
}

