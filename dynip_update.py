#!/usr/bin/env python3
"""
Dynamic DNS Updater for CloudFlare

This script updates CloudFlare DNS records with:
- Internal IPv4 address (RFC1918 private ranges)
- External IPv4 address (via DNS TXT query)
- External IPv6 address (via DNS TXT query)

All configuration is provided via environment variables.
"""

import os
import sys
import socket
import logging
import ipaddress
import dns.resolver
import requests
from typing import Optional, Dict, List

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class IPAddressDetector:
    """Detects various types of IP addresses for the current host."""

    RFC1918_RANGES = [
        ipaddress.ip_network('10.0.0.0/8'),
        ipaddress.ip_network('172.16.0.0/12'),
        ipaddress.ip_network('192.168.0.0/16'),
    ]

    @staticmethod
    def get_internal_ipv4() -> Optional[str]:
        """
        Get the internal IPv4 address (RFC1918 private address).
        Returns the first found private IP address.
        """
        try:
            # Get all network interfaces and their addresses
            import netifaces

            for interface in netifaces.interfaces():
                addrs = netifaces.ifaddresses(interface)
                if netifaces.AF_INET in addrs:
                    for addr_info in addrs[netifaces.AF_INET]:
                        ip_str = addr_info.get('addr')
                        if ip_str:
                            try:
                                ip = ipaddress.ip_address(ip_str)
                                # Check if it's in any RFC1918 range
                                for network in IPAddressDetector.RFC1918_RANGES:
                                    if ip in network:
                                        logger.info(f"Found internal IPv4: {ip_str}")
                                        return ip_str
                            except ValueError:
                                continue
        except ImportError:
            # Fallback method without netifaces
            logger.warning("netifaces not available, using fallback method")
            try:
                # Connect to a public IP to determine our interface address
                # This doesn't actually send packets
                s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
                s.connect(('8.8.8.8', 80))
                ip_str = s.getsockname()[0]
                s.close()

                ip = ipaddress.ip_address(ip_str)
                for network in IPAddressDetector.RFC1918_RANGES:
                    if ip in network:
                        logger.info(f"Found internal IPv4: {ip_str}")
                        return ip_str
            except Exception as e:
                logger.error(f"Error detecting internal IPv4: {e}")

        logger.warning("No internal IPv4 address found")
        return None

    @staticmethod
    def get_external_ipv4() -> Optional[str]:
        """
        Get external IPv4 address by querying o-o.myaddr.l.google.com via IPv4.
        """
        try:
            resolver = dns.resolver.Resolver()
            # Force IPv4 by creating a resolver that only uses IPv4 nameservers
            resolver.nameservers = ['8.8.8.8', '8.8.4.4']  # Google's IPv4 DNS

            answers = resolver.resolve('o-o.myaddr.l.google.com', 'TXT')
            for rdata in answers:
                # TXT records come back with quotes, strip them
                ip_str = rdata.to_text().strip('"')
                # Validate it's an IPv4 address
                ipaddress.IPv4Address(ip_str)
                logger.info(f"Found external IPv4: {ip_str}")
                return ip_str
        except Exception as e:
            logger.error(f"Error detecting external IPv4: {e}")

        return None

    @staticmethod
    def get_external_ipv6() -> Optional[str]:
        """
        Get external IPv6 address by querying o-o.myaddr.l.google.com via IPv6.
        """
        try:
            resolver = dns.resolver.Resolver()
            # Force IPv6 by using IPv6 nameservers
            resolver.nameservers = ['2001:4860:4860::8888', '2001:4860:4860::8844']  # Google's IPv6 DNS

            answers = resolver.resolve('o-o.myaddr.l.google.com', 'TXT')
            for rdata in answers:
                # TXT records come back with quotes, strip them
                ip_str = rdata.to_text().strip('"')
                # Validate it's an IPv6 address
                ipaddress.IPv6Address(ip_str)
                logger.info(f"Found external IPv6: {ip_str}")
                return ip_str
        except Exception as e:
            logger.error(f"Error detecting external IPv6: {e}")

        return None


class CloudFlareUpdater:
    """Handles CloudFlare DNS record updates via their API."""

    def __init__(self, api_token: str, zone_id: str):
        """
        Initialize CloudFlare updater.

        Args:
            api_token: CloudFlare API token
            zone_id: CloudFlare Zone ID
        """
        self.api_token = api_token
        self.zone_id = zone_id
        self.base_url = "https://api.cloudflare.com/client/v4"
        self.headers = {
            "Authorization": f"Bearer {api_token}",
            "Content-Type": "application/json"
        }

    def get_record_id(self, name: str, record_type: str) -> Optional[str]:
        """
        Get the DNS record ID for a given name and type.

        Args:
            name: Full DNS name (e.g., internal.example.com)
            record_type: Record type (A or AAAA)

        Returns:
            Record ID if found, None otherwise
        """
        url = f"{self.base_url}/zones/{self.zone_id}/dns_records"
        params = {
            "name": name,
            "type": record_type
        }

        try:
            response = requests.get(url, headers=self.headers, params=params)
            response.raise_for_status()
            data = response.json()

            if data.get('success') and data.get('result'):
                record_id = data['result'][0]['id']
                logger.debug(f"Found record ID {record_id} for {name} ({record_type})")
                return record_id
        except Exception as e:
            logger.error(f"Error getting record ID for {name}: {e}")

        return None

    def create_record(self, name: str, record_type: str, content: str, proxied: bool = False) -> bool:
        """
        Create a new DNS record.

        Args:
            name: Full DNS name
            record_type: Record type (A or AAAA)
            content: IP address
            proxied: Whether to proxy through CloudFlare

        Returns:
            True if successful, False otherwise
        """
        url = f"{self.base_url}/zones/{self.zone_id}/dns_records"
        data = {
            "type": record_type,
            "name": name,
            "content": content,
            "ttl": 120,  # 2 minutes for dynamic DNS
            "proxied": proxied
        }

        try:
            response = requests.post(url, headers=self.headers, json=data)
            response.raise_for_status()
            result = response.json()

            if result.get('success'):
                logger.info(f"Created {record_type} record for {name} -> {content}")
                return True
            else:
                logger.error(f"Failed to create record: {result.get('errors')}")
        except Exception as e:
            logger.error(f"Error creating record for {name}: {e}")

        return False

    def update_record(self, record_id: str, name: str, record_type: str, content: str, proxied: bool = False) -> bool:
        """
        Update an existing DNS record.

        Args:
            record_id: CloudFlare record ID
            name: Full DNS name
            record_type: Record type (A or AAAA)
            content: IP address
            proxied: Whether to proxy through CloudFlare

        Returns:
            True if successful, False otherwise
        """
        url = f"{self.base_url}/zones/{self.zone_id}/dns_records/{record_id}"
        data = {
            "type": record_type,
            "name": name,
            "content": content,
            "ttl": 120,
            "proxied": proxied
        }

        try:
            response = requests.put(url, headers=self.headers, json=data)
            response.raise_for_status()
            result = response.json()

            if result.get('success'):
                logger.info(f"Updated {record_type} record for {name} -> {content}")
                return True
            else:
                logger.error(f"Failed to update record: {result.get('errors')}")
        except Exception as e:
            logger.error(f"Error updating record for {name}: {e}")

        return False

    def delete_record(self, record_id: str, name: str, record_type: str) -> bool:
        """
        Delete a DNS record.

        Args:
            record_id: CloudFlare record ID
            name: Full DNS name
            record_type: Record type (A or AAAA)

        Returns:
            True if successful, False otherwise
        """
        url = f"{self.base_url}/zones/{self.zone_id}/dns_records/{record_id}"

        try:
            response = requests.delete(url, headers=self.headers)
            response.raise_for_status()
            result = response.json()

            if result.get('success'):
                logger.info(f"Deleted {record_type} record for {name}")
                return True
            else:
                logger.error(f"Failed to delete record: {result.get('errors')}")
        except Exception as e:
            logger.error(f"Error deleting record for {name}: {e}")

        return False

    def delete_record_if_exists(self, name: str, record_type: str) -> bool:
        """
        Delete a DNS record if it exists.

        Args:
            name: Full DNS name
            record_type: Record type (A or AAAA)

        Returns:
            True if record was deleted or didn't exist, False if deletion failed
        """
        record_id = self.get_record_id(name, record_type)

        if record_id:
            return self.delete_record(record_id, name, record_type)
        else:
            logger.debug(f"No {record_type} record found for {name} to delete")
            return True

    def upsert_record(self, name: str, record_type: str, content: str, proxied: bool = False) -> bool:
        """
        Create or update a DNS record.

        Args:
            name: Full DNS name
            record_type: Record type (A or AAAA)
            content: IP address
            proxied: Whether to proxy through CloudFlare

        Returns:
            True if successful, False otherwise
        """
        record_id = self.get_record_id(name, record_type)

        if record_id:
            return self.update_record(record_id, name, record_type, content, proxied)
        else:
            return self.create_record(name, record_type, content, proxied)


def get_env_or_exit(var_name: str, required: bool = True) -> Optional[str]:
    """
    Get an environment variable or exit if required and not found.

    Args:
        var_name: Environment variable name
        required: Whether the variable is required

    Returns:
        Variable value or None if not required and not found
    """
    value = os.getenv(var_name)
    if required and not value:
        logger.error(f"Required environment variable {var_name} not set")
        sys.exit(1)
    return value


def main():
    """Main execution function."""
    logger.info("Starting Dynamic DNS Updater")

    # Get configuration from environment variables
    cf_api_token = get_env_or_exit('CF_API_TOKEN')
    cf_zone_id = get_env_or_exit('CF_ZONE_ID')
    hostname = get_env_or_exit('HOSTNAME')

    # Optional: Different domains/subdomains for each type
    internal_domain = get_env_or_exit('INTERNAL_DOMAIN', required=False) or hostname
    external_domain = get_env_or_exit('EXTERNAL_DOMAIN', required=False) or hostname
    ipv6_domain = get_env_or_exit('IPV6_DOMAIN', required=False) or hostname

    # Optional: Proxied settings (default False for dynamic DNS)
    proxied = os.getenv('CF_PROXIED', 'false').lower() == 'true'

    # Detect IP addresses
    detector = IPAddressDetector()
    internal_ipv4 = detector.get_internal_ipv4()
    external_ipv4 = detector.get_external_ipv4()
    external_ipv6 = detector.get_external_ipv6()

    # Initialize CloudFlare updater
    cf_updater = CloudFlareUpdater(cf_api_token, cf_zone_id)

    success_count = 0
    total_count = 0

    # Update internal IPv4 record
    if internal_ipv4:
        total_count += 1
        if cf_updater.upsert_record(internal_domain, 'A', internal_ipv4, proxied):
            success_count += 1
    else:
        logger.warning("No internal IPv4 address found - deleting any existing record")
        total_count += 1
        if cf_updater.delete_record_if_exists(internal_domain, 'A'):
            success_count += 1

    # Update external IPv4 record
    if external_ipv4:
        total_count += 1
        if cf_updater.upsert_record(external_domain, 'A', external_ipv4, proxied):
            success_count += 1
    else:
        logger.warning("No external IPv4 address found - deleting any existing record")
        total_count += 1
        if cf_updater.delete_record_if_exists(external_domain, 'A'):
            success_count += 1

    # Update external IPv6 record
    if external_ipv6:
        total_count += 1
        if cf_updater.upsert_record(ipv6_domain, 'AAAA', external_ipv6, proxied):
            success_count += 1
    else:
        logger.warning("No external IPv6 address found - deleting any existing record")
        total_count += 1
        if cf_updater.delete_record_if_exists(ipv6_domain, 'AAAA'):
            success_count += 1

    # Report results
    logger.info(f"Completed: {success_count}/{total_count} records updated successfully")

    if success_count == total_count and total_count > 0:
        logger.info("All updates successful!")
        sys.exit(0)
    elif success_count > 0:
        logger.warning("Some updates failed")
        sys.exit(1)
    else:
        logger.error("All updates failed")
        sys.exit(1)


if __name__ == "__main__":
    main()
